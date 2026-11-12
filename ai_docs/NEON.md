# Neon Postgres

**Last updated:** 2026-11-12 · **Schema version:** v32 · **Needs:** Terraform >= 1.14.0,
`kislerdm/neon` 0.18.0, `cyrilgdn/postgresql` 1.27.0 (both pinned by meroku)

meroku can give an env a [Neon](https://neon.tech) Postgres project and give each ECS
service its own login role in one database. Each service gets `DATABASE_URL` from an SSM
SecureString. meroku never deletes a Neon project or database.

Neon support is opt-in. An env YAML without a `neon:` block renders the same `main.tf` as
before and needs no Neon login.

## Get started

You need:

- [ ] Terraform 1.14.0 or later (`terraform version`)
- [ ] A Neon account, and either `NEON_API_KEY` set or the Neon CLI logged in
- [ ] An env that already deploys with meroku

Log in once with the Neon CLI. It ships as `neon`; the older `neonctl` binary also works.

```bash
npm i -g neon
neon login         # opens a browser
```

Add a `neon:` block to `project/dev.yaml` and give the backend access:

```yaml
schema_version: 32

neon:
  mode: create
  project:
    name: myapp-dev
    org_id: "org-placeholder-000"     # from the Neon console, organization settings
    region_id: aws-us-east-1
    pg_version: 17
  databases:
    - name: app
      create: true

workload:
  backend_neon:
    database: app
    access: readwrite
```

Deploy as usual:

```bash
./meroku            # then choose Deploy for dev
```

The deploy runs in two steps (see [The two-step deploy](#the-two-step-deploy)). After it,
the backend container has:

```text
DATABASE_URL=postgresql://myapp_dev_backend_app_f0819dbc:<password>@ep-placeholder-000-pooler.us-east-1.aws.neon.tech/app?sslmode=require
```

Already familiar? Skip to [Troubleshooting](#troubleshooting).

## Auth: where the Neon token comes from

meroku looks for a token in this order:

| Source | When it is used | Expiry |
|---|---|---|
| `NEON_API_KEY` | set in meroku's environment at startup (CI) | unknown; used as is |
| Neon CLI login | no `NEON_API_KEY`, and `neon` or `neonctl` on `PATH` | read from the login file |

For the CLI login, meroku runs `CI=true neon me -o json`. That refreshes an expired token
and never opens a browser. meroku then reads the token from the first file that exists:

```text
$NEON_CONFIG_DIR/credentials.json                 # if NEON_CONFIG_DIR is set (only this one)
$NEONCTL_CONFIG_DIR/credentials.json              # else, if NEONCTL_CONFIG_DIR is set
${XDG_CONFIG_HOME:-~/.config}/neon/credentials.json
${XDG_CONFIG_HOME:-~/.config}/neonctl/credentials.json
```

Named CLI profiles and keyring logins are not read. A keyring-only login reports "not
logged in".

meroku never stores the token. At startup it copies `NEON_API_KEY` into memory and removes
it from its own environment, so no child process inherits it. The token reaches only the
`terraform` processes meroku starts.

**Token life.** One Terraform run needs at least 5 minutes of token life. meroku checks
before each `terraform` process. When less is left, it asks:

```text
⚠️  The Neon token has 3m12s left; one Terraform run needs 5m0s.
  [1] Wait for it to expire, then refresh automatically (≈3 min)
  [2] Continue anyway (3 min left)
  [3] Cancel
```

Choice 1 polls every 5 seconds until the token expires, then refreshes it with the CLI.

## YAML reference

### The env `neon:` block

```yaml
neon:
  mode: reuse                         # required: create | reuse
  project_id: "proj-placeholder-000"  # reuse: required. create: not allowed
  pg_version: 17                      # reuse: required, 14-18, checked against Neon at deploy
  project:                            # create: required. reuse: not allowed
    name: myapp-prod
    org_id: "org-placeholder-000"
    region_id: aws-us-east-1
    pg_version: 17                    # 15 | 16 | 17 | 18, default 17
  owner_role: neondb_owner            # default neondb_owner
  databases:                          # all on the project's default branch
    - name: app
      create: true                    # meroku creates it
    - name: analytics
      create: false                   # it exists; meroku uses it and never changes it
```

- `mode: create` makes a new project. `mode: reuse` uses an existing one by id.
- `owner_role` is the role meroku connects as. It owns every table, sequence and schema
  meroku serves. Run your migrations as this role.
- In create mode, `neondb` already exists. List it with `create: false` if a service uses it.
- Database names must match `^[a-z_][a-z0-9_]{0,62}$`, also for reused databases.

### Per-service access

The backend uses `workload.backend_neon`. A named service uses `services[].neon`.

```yaml
workload:
  backend_neon:
    database: app                 # required; must be listed in neon.databases
    schemas: [app, public]        # optional, default [public]
    access: readwrite             # required: readonly | readwrite
    password_version: 1           # optional, default 1; raise it to rotate the password

services:
  - name: reports
    neon:
      database: analytics
      access: readonly
```

A service with a `neon` block must not also set `DATABASE_URL` in its env variables. A
service with `enabled: false` keeps its role and SSM parameter.

The web UI writes the same keys. Its canvas has a **Neon Postgres** node with Project and
Databases tabs, and once `neon:` exists the backend and every service get a **Database** tab.

### Typical layouts

```yaml
# dev.yaml and prod.yaml: one project each (recommended; prod always alone)
neon:
  mode: create
  project: { name: myapp-prod, org_id: "org-placeholder-000", region_id: aws-us-east-1 }
  databases: [{ name: app, create: true }]
```

```yaml
# staging.yaml: shares the dev project, with its own database
neon:
  mode: reuse
  project_id: "proj-placeholder-000"
  pg_version: 17
  databases: [{ name: staging_app, create: true }]
```

Envs may share a project. Each env then uses its own database. Role names contain the env,
so roles of two envs never collide. A project is the trust boundary: its owner password is
project-wide, so anyone who can deploy one env of a project can reach every database in it.

## What meroku creates

For each service with a `neon` block, meroku manages these objects in the service's
database on the default branch:

| Object | Detail |
|---|---|
| Login role | one per (env, service, database); see [Role names](#role-names) |
| Password | 32 characters, generated by Terraform |
| `search_path` | set on the role to the service's `schemas` list |
| Schemas | each listed schema other than `public` that does not exist yet, owned by `owner_role`; never dropped |
| Grants | per the table below, on every listed schema |
| Default privileges | `FOR ROLE <owner_role>`, so tables the owner creates later are covered |
| SSM parameter | `/<env>/<project>/<service>/database_url`, SecureString |

ECS turns the SSM parameter into the container variable `DATABASE_URL`: a URL on the pooled
host (`-pooler` after the first host label) with `sslmode=require`. Read it with the AWS CLI:

```bash
aws ssm get-parameter --name /dev/myapp/backend/database_url --with-decryption \
  --query Parameter.Value --output text
# postgresql://myapp_dev_backend_app_f0819dbc:<password>@ep-placeholder-000-pooler.us-east-1.aws.neon.tech/app?sslmode=require
```

| access | database | schema | tables | sequences |
|---|---|---|---|---|
| `readonly` | CONNECT | USAGE | SELECT | SELECT |
| `readwrite` | CONNECT | USAGE | SELECT, INSERT, UPDATE, DELETE | USAGE, SELECT, UPDATE |

A service role owns nothing and has no DDL rights, so run migrations as `owner_role`. A
table owned by another role gets no grant, and its schema's grants then show as an update
on every plan. One way to get the owner's URL in CI:

```bash
export MIGRATE_URL="$(neon connection-string --project-id proj-placeholder-000 \
  --role-name neondb_owner --database-name app)"
psql "$MIGRATE_URL" -f migrations/001_orders.sql
```

You can check a role's rights as `owner_role`:

```sql
SELECT has_table_privilege('myapp_dev_backend_app_f0819dbc', 'app.orders', 'INSERT');
-- true for readwrite, false for readonly
SHOW search_path;   -- run as the service role: app, public
```

### Role names

```text
<project>_<env>_<service>_<database>_<8 hex>      # at most 63 bytes
myapp_dev_backend_app_f0819dbc
myapp_prod_backend_app_f96ca12b
```

The hex part is a SHA-256 of the four raw names. Renaming the env or the meroku project
gives every role a new name, so meroku drops the old roles and creates new ones.

### Rotating a password

Raise `password_version`:

```yaml
workload:
  backend_neon:
    database: app
    access: readwrite
    password_version: 2
```

The next deploy sets a new password on the role and writes a new `DATABASE_URL` to SSM.
Tasks read `DATABASE_URL` when they start. meroku passes each service's parameter version to
the workloads module (`ssm_dependencies`, one token per service). A token is unknown at plan
while its parameter is created or changes value, which defers only that service's SSM read to
apply, so the same apply rolls out a new task definition for that service and leaves every
other service alone. The rollout is not yet verified against a live deploy.

## The two-step deploy

The `postgresql` provider needs the project and databases to exist before it can connect.
meroku therefore deploys a Neon env in two steps, in one Terraform root and one state:

```text
release ─► step 1 ─► read state ─► owner credentials ─► step 2
  │          │                         │                   │
  │          │ plan -target module.neon │ from the Neon API │ full plan and apply:
  │          │ project + databases      │ (memory only)     │ roles, grants, SSM, ECS
  │          │ own plan file            │                   │
  └ objects that left the YAML leave state; nothing is deleted in Neon
```

1. **Step 1** plans and applies only the project and the databases. Its screen title is
   "Neon — step 1 of 2: project and databases". With no changes it prints
   `✅ Neon step 1: the project and databases need no changes.` and goes on.
2. meroku reads the owner role's password and the direct host from the Neon API.
3. **Step 2** is the normal plan and apply, titled "Step 2 of 2: everything else" when step 1
   applied. The owner credentials reach Terraform only through the ephemeral variable
   `TF_VAR_neon_pg_credentials`, so they are not in `main.tf`, the state or the plan file.

The replace key `[r]` is hidden on every plan screen of a Neon env.

meroku needs Terraform >= 1.14.0 for an env with `neon:` or with Neon objects in its state.
The pre-flight stops with:

```text
Terraform version 1.13.4 is installed, but minimum required version is 1.14.0
```

### Running Terraform by hand

A plain `terraform plan` in `env/<env>/` has no Neon credentials. Its `postgresql`
providers get placeholder values and fail to connect. Run plans through meroku.

`terraform init -upgrade` is allowed in a Neon env, so meroku can still move
`hashicorp/aws` to a new major. `kislerdm/neon` (0.18.0) and `cyrilgdn/postgresql`
(1.27.0) are declared with exact versions in `main.tf`, in the modules and in
`neon_retired_override.tf`, so the upgrade has no other release of them to choose. When the
lock file lists one of them and the root configuration does not pin it, meroku refuses the
upgrade and names the provider:

```text
init -upgrade refused: it could move a Neon provider off its pinned release; see NEON.md (kislerdm/neon not pinned in main.tf)
```

## meroku never deletes Neon data

meroku deletes only what it made inside Postgres and AWS: service roles, their grants and
default privileges, and SSM parameters. It never deletes a Neon project or database and never
drops a schema, and every project or database it created carries `prevent_destroy = true`.

| You do this | What happens |
|---|---|
| Remove a created database from the YAML, or rename it | It leaves Terraform state and goes in the Kept file; its roles are dropped |
| Set `create: false` on a created database, or switch the project to `mode: reuse` | It leaves state and stays in use as data; meroku prints `No longer managed by meroku (kept in Neon, now used as data): …` |
| Remove the `neon:` block, or destroy the env | Every created object leaves state and goes in the Kept file; every role is dropped |
| Destroy without a Neon login | Every role and every created object stays in Neon and goes in the Kept file |
| Change `org_id`, `region_id`, `pg_version` or `owner_role` of a created project | The plan fails; see [the refusal message](#neon-this-change-would-replace--and-delete-its-data) |
| Run `terraform destroy` by hand | Terraform refuses on `prevent_destroy` |

Before a release, meroku prints what leaves state:

```text
🔓 Released from Terraform state (nothing is deleted in Neon):
   module.neon.neon_database.created["reports_old"]
```

### The Kept file

meroku records every released object in `.meroku/neon-kept/<env>.json` under the project
root. The file holds ids and names only (kind, org id, project id, branch id, database or
role name, release time). Its mode is `0600`, and `.meroku/` is gitignored.

The file is per checkout. Another checkout does not see it, but V-N20 prints the delete
command from the Neon API when a kept database blocks a deploy there.

meroku prints the report at the release, at the end of the deploy or destroy, and at the
start of every later deploy or destroy of that env:

```text
Kept in Neon — meroku never deletes Neon data. To delete it, run by hand. This deletes the
data permanently, and meroku does not check whether another env uses it:
  neon projects delete proj-placeholder-000   # project
  neon databases delete reports_old --project-id proj-placeholder-000 --branch br-placeholder-000   # database
  neon roles delete myapp_dev_reports_analytics_010562fd --project-id proj-placeholder-000 --branch br-placeholder-000   # role
To use it again instead, set mode: reuse with project_id: proj-placeholder-000, and create: false on
its databases.
```

A database or role inside a kept project is left out of the report; deleting the project
deletes them. The command name is the installed CLI (`neon`, else `neonctl`).

Read or clear the file from the project root:

```bash
meroku neon kept dev
# Nothing kept in Neon for dev.

meroku neon kept dev --forget
# Forgot the Neon objects kept for dev. They are still in Neon.
```

An entry also leaves the file when the YAML uses the object again as data, or when a run
with a Neon login proves it is gone from Neon.

## Limits and caveats

**Owner password in state (create mode).** For a project meroku creates, `kislerdm/neon`
0.18.0 stores the owner password in the `neon_project` resource state, and so in every plan
file of that env. This is the provider's behaviour, and reuse mode, which has no
`neon_project` resource, does not store it.

The S3 state backend is encrypted. meroku deletes `neon-step1.tfplan` and `tfplan` on every
exit path, and you keep `*.tfplan` out of git:

```bash
git check-ignore -v env/dev/tfplan || echo '*.tfplan' >> .gitignore
```

**PUBLIC access.** On databases meroku creates, it revokes PUBLIC's `CONNECT` and
`TEMPORARY`, so a service role reaches only its own database and cannot create temporary
tables. meroku does not change reused databases, so PUBLIC keeps both rights there.
Check:

```sql
SELECT datacl FROM pg_database WHERE datname = 'app';
-- created by meroku: no "=Tc/..." entry (PUBLIC); neondb_owner and neon_superuser keep CTc
```

Members of `neon_superuser` can still connect to created databases. Every role made in the
Neon Console, API or CLI is such a member. A role you create with SQL is not, and it needs
an explicit grant before it can connect to a created database:

```sql
GRANT CONNECT ON DATABASE app TO analytics_reader;
```

**Schema changes and the pooler.** A change to a service's `schemas` updates the role's
`search_path`. Pooled connections keep the old value until Neon's pooler recycles its
server connections. After such an apply meroku prints the roles. To apply the change now,
connect to the database as `owner_role` and end the role's sessions:

```sql
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = 'myapp_dev_backend_app_f0819dbc';
```

**`DROP ROLE` and temporary tables.** In a reused database, a service can hold temporary
tables in a long-lived pooled session, and `DROP ROLE` then fails with SQLSTATE 2BP01. List the
sessions, end them as `owner_role`, then run meroku again. Created databases are not
affected.

```sql
SELECT pid, application_name, backend_start FROM pg_stat_activity
 WHERE usename = 'myapp_dev_backend_app_f0819dbc';
SELECT pg_terminate_backend(pid) FROM pg_stat_activity
 WHERE usename = 'myapp_dev_backend_app_f0819dbc';
```

**Default branch only.** meroku uses the project's default branch, which it reads and never
creates or changes. There is no branch config. If the default branch changes
outside meroku, the deploy stops with V-N39 until you set it back.

**Moves and removals.** When a service moves to another database or loses its `neon`
block, its old role is dropped in the same apply that rolls out the new task definition.
Tasks still on the old URL can lose access during that rollout.

**Secrets in child processes.** Terraform `local-exec` provisioners run under
`env -u NEON_API_KEY -u TF_VAR_neon_pg_credentials`. The Fix-with-AI pane and its printed
fallback command drop both variables too. A `TF_LOG` or `TF_LOG_PATH` you set yourself
writes provider logs outside meroku's redaction.

**Other limits.**

- Scheduled and event tasks get no Neon access.
- Reuse mode accepts a PG 14 project but refuses any service access to it (V-N28).
- `pg_version` for a created project is 15, 16, 17 or 18.
- meroku never detects a password that someone changed outside it; raise `password_version` and deploy to set a new one.

## Troubleshooting

A failed rule prints as `neon: <path>: <problem> — <fix> [V-Nx]`. At `meroku generate` and
at the start of a deploy, meroku checks the YAML alone and lists every generate-time problem
in one error, so you can fix them in one pass:

```text
invalid Neon settings:
  neon: neon.mode: mode is missing — set mode: create (meroku creates the project) or mode: reuse (an existing project) [V-N1]
```

### Generate-time rules

| Rule | Message (problem part) | Fix |
|---|---|---|
| V-N1 | `mode is missing` | set `mode: create` or `mode: reuse` |
| V-N2 | `reuse mode needs the id of an existing Neon project …` | set `project_id`; list ids with `neon projects list` |
| V-N2 | `reuse mode needs the project's Postgres major version` | set `pg_version` to 14–18 |
| V-N3 | `create mode makes a new project, so it takes no project_id` | remove `project_id`, or use `mode: reuse` |
| V-N8 | `database "app" is listed twice` | list each database once |
| V-N9 | `a new Neon project already comes with the neondb database` | set `create: false` on `neondb` |
| V-N10 | `an empty list grants nothing` | list a schema, or leave `schemas` out |
| V-N11 | `database "app" is not declared in neon.databases` | add it to `neon.databases` |
| V-N13 | `a Neon database is requested but the env has no top-level neon: block` | add `neon:`, or remove the service block |
| V-N14 | `the service also sets DATABASE_URL in its env variables …` | remove `DATABASE_URL` from the env variables |
| V-N15 | `… is not a Postgres role name` | fix `owner_role` |
| V-N16 | `its role name … is the same as …'s` | rename one of the services |
| V-N28 | `Postgres 14 lets every role create tables in public …` | upgrade the project to 15+, or remove the service blocks |

### Deploy-time rules

These rules check the YAML against Neon and the Terraform state. Most run before step 1;
V-N21 runs again after it, and V-N23, V-N24 and V-N25 run only after step 1. A failing check
stops the deploy and prints one indented line per problem under
`neon: stopped before anything was changed:`.

Sample lines, one per rule:

```text
  neon: neon.databases[app]: app already exists in Neon. meroku will not adopt existing data — set create: false, or delete it first: neon databases delete app --project-id proj-placeholder-000 --branch br-placeholder-000 [V-N20]
  neon: neon.project_id: project proj-placeholder-000 does not exist in Neon, or this login cannot see it — check project_id and the Neon login [V-N21]
  neon: neon.databases[analytics]: database analytics does not exist on the default branch of proj-placeholder-000 — create it in Neon, or set create: true [V-N21]
  neon: neon.databases[app]: database app is owned by app_owner, not by owner_role neondb_owner; grants by a non-owner do nothing — set owner_role: app_owner, or make neondb_owner the owner of app [V-N23]
  neon: neon: branch br-placeholder-000 of proj-placeholder-000 has no read_write endpoint — add a read_write compute endpoint to the default branch in Neon [V-N24]
  neon: neon.pg_version: pg_version is 16, but project proj-placeholder-000 runs Postgres 17 — set pg_version: 17 [V-N33]
  neon: neon.project.name: a project named myapp-dev (proj-placeholder-000) already exists in org-placeholder-000 — set mode: reuse with project_id: proj-placeholder-000, or choose another name [V-N35]
  neon: neon.owner_role: owner_role is app_owner, but the roles of database app were created by neondb_owner — set owner_role back to neondb_owner, or remove the services' neon blocks and deploy that change first [V-N36]
```

Each line names its own fix. Some rules need more context:

- **V-N20** usually means a database meroku kept earlier. Set `create: false` to use it as
  data, or delete it with the printed command.
- **V-N21** for a project: check `project_id`, and check which account the CLI is logged
  in to with `neon me`.
- **V-N25**: the owner password cannot be revealed. Check `owner_role` and that the login
  may read the project's roles.
- **V-N26**: another SSM parameter under the service's path ends in `database_url` in some
  letter case and would also become `DATABASE_URL`. Delete or rename it.
- **V-N38**: an env cannot move to another project in one deploy. Revert, or remove
  `neon:` and deploy first (meroku keeps the old project), then add the new one.

**V-N39: the default branch changed.** meroku's roles and databases sit on the branch
recorded in state. Set that branch as the default again, then rerun meroku:

```text
neon: neon: the default branch of proj-placeholder-000 is now br-other-000, but meroku's roles and databases are on br-placeholder-000 — Make br-placeholder-000 the default branch again (`neon branches set-default br-placeholder-000 --project-id proj-placeholder-000`), then run meroku again. [V-N39]
```

```bash
neon branches set-default br-placeholder-000 --project-id proj-placeholder-000
```

**V-N29: a created object was deleted outside meroku.** An interactive deploy asks:

```text
⚠️  database app was created by meroku but has been deleted outside it.
  [1] Create it again (empty)
  [2] Cancel
```

A non-interactive run stops with the same text and `[V-N29]`.

### neon: this change would replace … and delete its data

```text
neon: this change would replace module.neon.neon_project.this[0] and delete its data. meroku never deletes Neon data. Revert neon.project.org_id, neon.project.region_id, neon.project.pg_version and neon.owner_role. To start over, remove it from dev.yaml and deploy (meroku keeps it and prints the command to delete it), delete it, then add it back.
```

A field changed that would make Terraform replace a created project or database, and the
`prevent_destroy` lifecycle rule stopped the plan. Revert the field the message names.

### No Neon credentials

```text
❌ Neon is enabled for dev, but meroku found no Neon credentials.
   Either set NEON_API_KEY (for CI), or log in with the Neon CLI:
     npm i -g neon && neon login
```

Neither `NEON_API_KEY` nor a Neon CLI is available. Install the CLI and log in, or export
the key before you start meroku. When the CLI exists but is not logged in, meroku offers
`Log in now (neon login — opens a browser)`.

### Destroy without a Neon login

The destroy menu offers `Destroy without Neon: forget the Neon roles in state — they stay
in Neon, listed` only when `main.tf` has no `module "neon"`. Otherwise it prints:

```text
To destroy without a Neon login, remove `neon:` from dev.yaml, run `meroku generate dev`, then destroy again.
```

### Postgres refused to drop a role (2BP01)

```text
neon: Postgres refused to drop myapp_dev_backend_app_f0819dbc (SQLSTATE 2BP01): open sessions of the role still hold objects, such as temporary
tables kept alive by pooled connections. Connect to that database as neondb_owner and run:
  SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = 'myapp_dev_backend_app_f0819dbc';
then run meroku again.
```

### this command needs a Neon login

`neon: this command needs a Neon login; run it from deploy or destroy` means a Terraform
command that touches Neon ran outside a deploy or destroy. Start it from the meroku menu.

## Code

| Area | Files |
|---|---|
| YAML rules, render input, role names | `app/neon_config.go`, `app/migration_v32.go` |
| Auth and token life | `app/neon_auth.go`, `app/child_env.go` |
| State, API and YAML reconciliation (V-N20…V-N39) | `app/neon_resolve.go`, `app/neon_api.go` |
| Two-step deploy, destroy prep | `app/neon_deploy.go`, `app/neon_retired.go` |
| Kept file, refusal and 2BP01 messages | `app/neon_keep.go` |
| Terraform | `modules/neon`, `modules/neon_access`, `modules/neon_ssm`, `env/main.hbs` |
| Web UI | `web/src/components/NeonNodeProperties.tsx`, `web/src/components/ServiceDatabaseTab.tsx`, `app/api_neon.go` |

## Next steps

- [YAML schema migrations](./MIGRATIONS.md): the v32 row.
- [Claude Code handoff](./CLAUDE_CODE_HANDOFF.md): how the Fix-with-AI pane runs.
- [Neon CLI reference](https://neon.tech/docs/reference/neon-cli) for the delete commands.
