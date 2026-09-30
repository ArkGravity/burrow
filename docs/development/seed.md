# Seed and default roles

Run `make migrate` and then `make seed`. The seed command is explicit,
noninteractive and obtains credentials from YAML or environment overrides.
It never prints a password, and the database stores an Argon2id hash. A newly
created administrator must change the temporary password on first login.

Seed uses a database transaction and the same serialization lock as authorization
changes. Repeating it supplements missing built-in roles/permission bindings,
preserving existing administrator credentials, profile, state and role assignments.
It rejects a bootstrap username belonging to an ordinary account and does not
create a new administrator under a renamed username in a populated database.
Name/code collisions with existing custom roles are reported instead of adopting
the custom role. Applied migration files are unchanged.

Compose runs `migrate`, then `seed`, then `app`. All use the same image. For a new
production database, configure an independent initial password through
`BURROW_BOOTSTRAP_ADMIN_PASSWORD`; seed rejects the public development example.
An existing administrator's password is not validated against or replaced by the
bootstrap password on a repeat seed.

## Default roles

| Role          | Permissions                                                                                                                                                     |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Administrator | Administrator behavior based on the `admin` ID; all current permissions and APP access, with normal account/APP/source checks                                   |
| Editor        | `dashboard:read`, `users:read`, `groups:read`, `roles:read`, `permissions:read`, `applications:read`, `applications:write`, `providers:read`, `providers:write` |
| Viewer        | No management permissions; authenticated users can access their own profile and their authorized application portal                                             |

Built-in roles cannot be edited or deleted. The built-in flag does not grant
administrator privilege. Editor maintains security-sensitive client and Provider
configuration and should be assigned only to trusted operators. It cannot reset
passwords, link external identities, manage users/groups or change authorization.

APP access is assigned through ordinary roles and group roles. Do not grant APP
login to the built-in Viewer role: every newly created user would otherwise gain
the same application. Built-in roles are excluded from APP login assignments.
Viewer users may receive additional explicit roles; effective permissions remain
the union of their direct roles and group roles.

## New-user defaults and group display

The creation form preselects Viewer. The API applies Viewer only when `roleIds`
is omitted on creation. An explicit empty array or another role list is respected,
and updates do not restore Viewer automatically. The implicit default is permitted
for an operator with `users:write`; explicit assignments still require
`authorization:write`. Existing users are not backfilled.

The Users list includes group summaries (`groups: [{id, name}]`) and existing
`groupIds`. Memberships and role IDs are loaded in batches for the current page.
Summaries are available under `users:read` without granting access to the entire
Groups resource. They do not expose group member lists or group role assignments.
The table displays up to three group tags, with `+N` and a tooltip for additional
groups, or an em dash for users without a group.
