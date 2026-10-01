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

## Roles and application grants

Administrator (`admin`) is the only seeded built-in role. Its ID grants
administrator behavior, subject to account/application/source checks. The built-in
flag alone does not grant administrator privilege. Administrator cannot be edited
or deleted.

Other roles are ordinary, editable permission sets created by operators with
`authorization:write`. Application access follows user/group → role → application
login permission. A single role can include one application or several explicitly
selected applications. New applications do not automatically grant access to
existing roles. Burrow application grants do not assign business roles in Grafana,
Harbor or other applications.

The permission code remains `app:<application ID>:login`. The management list and
role selector display the current application name, login action and Client ID.
Names may change or repeat without changing grants; Client ID distinguishes
applications with the same display name. The permissions API returns a compact
`application: {id, name, clientId}` reference under `permissions:read`, loaded in a
batch without requiring `applications:read` or exposing secrets/client settings.
Search supports application names, Client IDs and permission codes.

## Upgrade from Editor and Viewer

Migration `003_custom_roles.sql` upgrades schema v2 to v3 in a transaction:

- Remove user/group assignments to the legacy built-in Viewer when it has no
  permissions, preserving the authenticated profile/portal behavior.
- Remove unused legacy built-in Editor/Viewer roles and their permission links.
- Preserve assigned legacy roles with actual permissions as ordinary roles,
  retaining IDs, permissions and user/group assignments. Preexisting custom roles
  named Editor/Viewer are not changed.
- Record a `roles:migrate` policy event with the migration. Seed does not recreate
  either role or supplement their former permissions.

Back up the database and retain its master key before upgrading. Stop the old
server, migrate with the new binary, seed, then start the new server. Older binaries
reject schema v3; use the pre-upgrade backup for rollback.

## New-user defaults and group display

The creation form does not preselect a role. Omitted `roleIds` and an explicit empty
array both create a user with no roles; updates never add default roles. A valid
session grants access to personal resources and the authorized application portal,
even without roles. No application access is implicit. Creating a user without
role fields needs `users:write`; explicit authorization fields still require
`authorization:write`.

The Users list includes group summaries (`groups: [{id, name}]`) and existing
`groupIds`. Memberships and role IDs are loaded in batches for the current page.
Summaries are available under `users:read` without granting access to the entire
Groups resource. They do not expose group member lists or group role assignments.
The table displays up to three group tags, with `+N` and a tooltip for additional
groups, or an em dash for users without a group.
