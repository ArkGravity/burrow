# SAML IdP adapter

Burrow implements a deliberately small downstream SAML 2.0 IdP alongside OIDC. It uses Burrow passwords, the shared session, the global MFA policy and the existing `app:<application ID>:login` permission. The independently configured service provider owns its session and business permissions.

| Capability     | Contract                                                                                   |
| -------------- | ------------------------------------------------------------------------------------------ |
| Entry          | SP-initiated, signed HTTP Redirect AuthnRequest                                            |
| Signature      | RSA-SHA256; Response and Assertion both signed, SHA-256 digest                             |
| Response       | HTTP POST to one exactly registered ACS                                                    |
| Identity       | persistent NameID = immutable Burrow User ID                                               |
| Attributes     | `username`, `displayName`, `email`                                                         |
| SP trust       | Manually imported XML; one entity, one POST ACS, one RSA signing certificate               |
| Authentication | Password, forced password change and optional global TOTP MFA                              |
| ForceAuthn     | A new session must be bound to this exact SAML request                                     |
| Unsupported    | IdP-initiated login, POST AuthnRequest, encryption, SLO, groups/roles, SCIM, upstream SAML |

The SDK is `github.com/zitadel/saml v0.4.1`. The adapter uses its public XML/signature API rather than mounting all of its handlers. `goxmldsig v1.6.0` fixes the vulnerable default transitive version; see [dependency evidence](dependencies.md#saml-编解码门槛2026-10-20).

## Routes and configuration

| GET route                 | Result                                                             |
| ------------------------- | ------------------------------------------------------------------ |
| `/saml/metadata`          | EntityDescriptor; entity ID = configured issuer + `/saml/metadata` |
| `/saml/certificate`       | Current public IdP signing certificate, PEM                        |
| `/saml/sso`               | Signed Redirect request from a registered SP                       |
| `/saml/login?requestId=…` | Browser-bound internal continuation                                |

No route derives the issuer from Host or forwarded headers. Unsupported `/saml` paths return 404 and do not serve the SPA. Cookies retain the existing host-only, HttpOnly, SameSite=Lax policy, and are Secure in production.

Create an Application with `protocol: "saml"` and `saml: {metadataXml: "…"}`. Only the XML is writable; entity ID, ACS, certificate and config version are derived. Existing applications default to OIDC and the protocol is immutable. SAML has no client secret or PKCE fields. A random internal ClientID satisfies the original schema but is hidden from SAML API output and rejected by OIDC lookups.

Application metadata is returned to application managers, not personal portals. Permission references include the protocol and SP entity ID through a batched lookup. Metadata changes increment a config version and invalidate older pending requests atomically with the administrative audit.

## Request and signing boundaries

Raw URLs are limited to 32 KiB; inflated XML to 256 KiB and depth 32; RelayState to 80 bytes. Duplicate query parameters, duplicate XML IDs/attributes, DTD/entity declarations and unsupported algorithms are rejected. Signature verification reconstructs the signed query from its original encoded values. XML is never fetched from a supplied URL.

Only a verified request can select a registered ACS. Destination, version, request ID, time, POST binding and exact ACS or registered ACS index are checked. NameIDPolicy may be absent or persistent. IsPassive produces signed `NoPassive`; unsupported context produces `NoAuthnContext`; unsupported NameID policy produces `InvalidNameIDPolicy`. These capability failures are persisted so retrying after an internal failure cannot turn rejection into success.

For received time R and request IssueInstant I:

```text
R - 5m <= I <= R + 60s
ExpiresAt   = min(R + 5m, I + 6m)
ReplayUntil = max(ExpiresAt, I + 6m)
```

The independent `(SP entity ID, SP request ID)` replay guard has no user/application foreign key. Cancel, reset, metadata/key rotation and deletion/recreation of an application cannot shorten its life. It is created atomically with the internal request; consumption never extends either deadline.

Final signing rechecks the enabled user/application, shared session, authentication version, MFA, permissions, browser binding and config version under the same lock as administrative mutations. AuthnInstant is the session's real authentication time, not signing time; assertion expiry is at most three minutes and never exceeds the shared session. SessionIndex is independently random and does not expose Burrow session IDs or credentials.

The signed form is buffered before marking the request consumed. Consumption and audit commit together; the form is written only after commit. Failure writes no assertion/form and leaves an unconsumed request available to the same browser. HTML escaping, no-store, no-referrer and a nonce-based CSP protect the POST form. Logs and audits do not contain raw requests, responses or RelayState.

## Persistence and upgrade

Migration 006 adds the protocol and typed login continuation fields, four SAML tables and a session UTC marker. It leaves migrations 001–005 unchanged and preserves OIDC credentials, grants, sessions, tokens, pending authorizations and signing keys.

New sessions and SAML transactions persist UTC timestamps. The marker preserves historical PostgreSQL session expiry comparisons instead of reinterpreting old wall-clock timestamps or extending their life. SQLite compares absolute instants. SAML asks an older session to authenticate again, ensuring its AuthnInstant is unambiguous; existing OIDC sessions retain access under the original policy.

`migrate` explicitly initializes an encrypted RSA-3072 SAML key and a 365-day signing certificate, idempotently. Serving requests never generates keys. Readiness requires a usable active key when an enabled SAML application exists. Rotation is independent from OIDC and requires an audit reason:

```bash
burrow keys-rotate --config configs/config.local.yaml --protocol saml --reason "Scheduled SAML certificate renewal"
```

See the [Jenkins guide](../../examples/local-sso/jenkins/README.md), [recovery procedure](../operations/recovery.md#saml-证书与-schema-v6) and [actual verification record](../testing/saml-jenkins.md).
