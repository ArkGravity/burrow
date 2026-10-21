# Jenkins SAML onboarding

Use the [complete local Jenkins guide](../../examples/local-sso/jenkins/README.md) for the fixed image/plugin versions, manual configuration, ordinary Burrow roles, Jenkins Matrix permissions and controlled recovery.

Burrow acts as the IdP and Jenkins as the SP. The stable Burrow user ID is the persistent NameID; leave Jenkins Username Attribute blank. Applications retain their own sessions. The [adapter boundary](../development/saml-adapter.md) defines supported requests, signatures, expiry and replay handling. Check the [verification record](../testing/saml-jenkins.md) before treating a local example as tested interoperability.
