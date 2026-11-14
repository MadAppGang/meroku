# Vaults

A vault is a set of secrets several workloads share. The YAML declares the
vault and the names of its secrets; the values live only in SSM Parameter Store,
under `/<env>/<project>/vault/<name>/`, and are written from the web UI's
**Vaults** tab. Each declared secret reaches every connected workload as a
container secret, without copying parameters between workloads' own paths and
without custom Terraform. Built by `modules/vaults`.

## YAML

```yaml
vaults:                                   # → /<env>/<project>/vault/<name>/
  - name: shared
    secrets: [api_key, database_url]      # required; the values are not here
  - name: payments
    secrets: []                           # a vault with nothing in it yet

workload:                                 # the backend
  vaults:
    - name: shared

services:
  - name: api
    vaults:
      - { name: shared, access: read }
      - { name: payments, access: read_write, prefix: PAY }  # database_url → PAY_DATABASE_URL

scheduled_tasks:
  - name: nightly
    vaults: [{ name: shared }]           # access defaults to read

event_processor_tasks:
  - name: ingest
    vaults: [{ name: payments, prefix: PAY }]
```

- `vaults[].name`: 1 to 32 lower-case letters, digits, hyphens and underscores,
  starting and ending with a letter or a digit. Unique.
- `vaults[].secrets`: required, `[]` allowed. Each name is 1 to 63 lower-case
  letters, digits and underscores, starting with a letter
  (`^[a-z][a-z0-9_]{0,62}$`), listed once. Lower case only, so two names can
  never become the same variable.
- A connection's `name` must be a vault of the top-level list, and a workload
  connects to a vault at most once.
- `access`: `read` (the default) or `read_write`.
- `prefix`: optional; upper-case letters, digits and underscores, starting with a
  letter. It goes before every secret name from that vault, joined with `_`.

`meroku generate` refuses anything else and names the field, and refuses the
collisions listed under [Name collisions](#name-collisions-what-is-refused-and-when).

A vault without a `secrets` key is refused:

```
vault "shared" has no secrets list: list its secret names under vaults[0].secrets;
the Vaults tab can import the names already in SSM
```

That is the shape v6.0.0 wrote; see
[Moving a v6.0.0 vault](#moving-a-v600-vault). There is no schema migration,
because a migration reads only the YAML and cannot know the names in SSM
([MIGRATIONS.md](./MIGRATIONS.md)).

The web UI has a **Vaults** node on the canvas, always shown, which adds and
removes vaults and manages each vault's secrets; removing a vault also removes
every workload's connection to it. Each workload panel (backend, service,
scheduled task, event task) has a **Vaults** tab to connect a vault, pick read
or read-write and set a prefix.

## Secret names

A declared secret `database_url` of vault `payments` is the parameter
`/dev/myapp/vault/payments/database_url` and becomes the container secret

- `DATABASE_URL` for a workload connected without a prefix;
- `PAY_DATABASE_URL` for one connected with `prefix: PAY`.

The task definition's `valueFrom` is the parameter's ARN. The names come from
the YAML list alone, so they are known at plan on every plan, a new environment
included, and nothing waits on an SSM read. Secrets are sorted by name, so the
order of the YAML never changes a task definition.

A parameter under the vault's path that the list does not declare is no
workload's secret. Nor is anything deeper (`…/vault/payments/old/database_url`).

## Values: write-only, from the Vaults tab

meroku never stores a value, and nothing returns one. The Vaults tab shows, per
vault, every declared secret and every parameter found under the path:

| Row | Shown | Actions |
|---|---|---|
| declared, set | name → variable name, version, last modified | Replace value, Remove |
| declared, not set | "No value yet" | Set value |
| in SSM, not declared | "Not in the YAML" | Add to YAML, Delete from AWS |
| removed in this session, not applied | warning: the deployed workloads still use it until you apply | Add to YAML, Delete from AWS |

Removing a name from the YAML changes nothing in AWS until an apply: the
deployed task definitions still reference the parameter, and a new task fails to
start once its value is gone. So for a name the YAML listed when the tab opened,
or that was removed there, the row and the delete dialog say to apply first,
then delete. The delete stays available. A `warning` in the DELETE answer is
shown under the vault.

The value inputs are password fields, always empty. Adding a secret adds its
name to `vaults[i].secrets` (an ordinary config change) and writes the value
once the YAML is saved; the name and value fields clear only once AWS has the
value, so a refused write can be sent again. A new name that AWS already has
but the YAML does not list is refused in the form: use Add to YAML on its row,
then Replace value. The tab talks to `/api/vaults/secrets`
(`app/api_vault_secrets.go`):

| Request | Does | Answer |
|---|---|---|
| `GET ?env&vault` | lists the YAML names and the parameters directly under the path (`DescribeParameters`, metadata only) | `{vault, path, secrets: [{name, envName, declared, set, type?, version?, lastModified?}]}`, sorted, never null |
| `PUT ?env&vault` `{name, value}` | `PutParameter`, `SecureString`, overwrite | `{name, version}`; 409 unless the saved YAML declares the name |
| `DELETE ?env&vault&name` | `DeleteParameter` | `{name, deleted: true}`, plus a `warning` for a declared name; 404 if there is no value |

The server builds the path from the env YAML's `env`, `project` and a declared
vault; the browser never sends one, and a YAML without `project` is refused
(400). It uses the env's own `aws_profile` and `region`; when either is absent
the SDK's default chain (`AWS_PROFILE`, `AWS_REGION`, the default profile) fills
it in. When the YAML has `account_id`, every request first asks
`sts:GetCallerIdentity` and refuses (403, naming both accounts) credentials of
another account, before any SSM call. Its SSM client has only
`DescribeParameters`, `PutParameter` and `DeleteParameter`, so it cannot read a
value. A value is 1 to 4096 bytes, the standard-tier limit, counted in UTF-8
bytes on both the server and the tab.
DELETE also accepts an upper-case or dotted name, so a parameter written before
names were declared can be cleaned up.

From a terminal, the same write is:

```sh
aws ssm put-parameter --type SecureString --overwrite \
  --name /dev/myapp/vault/payments/database_url \
  --value '<value>'
```

Keep vault parameters `SecureString`: the plan-time check below returns the value of a `String` or `StringList` parameter in plaintext into the plan.

### A secret with no value yet: a warning at plan

For every connected vault that declares a secret, the plan lists the
parameters under the vault's path (`with_decryption = false`) and
warns about each declared name with no parameter:

```
Warning: Check block assertion failed
Vault "payments" has no value yet for database_url (under /dev/myapp/vault/payments/).
Set the values in the web UI's Vaults tab. Until then, a new task of a workload
connected to this vault cannot start.
```

It is a `check` block (`modules/vaults/value_check`), so it never fails a plan
or an apply, and an error reading SSM is a warning too. The apply registers the
task definitions anyway; ECS refuses to start a task whose secret has no
parameter. Set the value and the next task starts.

### Changing a value needs a redeploy

Adding or removing a declared name changes the connected task definitions on
the next apply. Changing a value does not: the task definition names the
parameter, not its value, and meroku's deployment Lambda only reacts to
parameter changes under a workload's own path. A running task keeps the value
it started with. Redeploy each connected workload (or let its tasks be replaced)
to pick up a new value.

### Removing a secret

Remove the name from the list (the tab's **Remove**), apply, then delete the
value. In that order no running task definition names a missing parameter.
Deleting the value first leaves every connected workload's current revision
pointing at it: its new tasks fail to start until the value is set again or the
removal is applied. The DELETE answer for a declared name says so.

### Destroy keeps the values

Terraform never creates, reads or deletes a vault parameter, so `meroku
destroy` leaves every value in SSM. Delete them from the tab, or with
`aws ssm delete-parameter`, when the environment is gone for good.

## Moving a v6.0.0 vault

In v6.0.0 a vault had no `secrets` key and every parameter directly under its
path became a secret. `meroku generate` now refuses such a vault. To move it:

1. Open the Vaults tab. The vault shows "meroku needs this vault's secret
   names".
2. **Import names from AWS** adds every name found under the path to
   `vaults[i].secrets` (or **Start empty** writes `secrets: []`).
3. Generate and apply. The task definitions are unchanged except for
   `valueFrom`, which is now the parameter's ARN rather than its name, so each
   connected workload gets one new revision.

Names v6.0.0 accepted but the list does not (upper case, hyphens, dots) cannot
be imported as they are. Write the value again under a lower-case name from the
tab, update the application if the variable name changes, and delete the old
parameter.

By hand, the same is `aws ssm describe-parameters --parameter-filters
Key=Path,Option=OneLevel,Values=/dev/myapp/vault/shared` and copying the last
segments into the list.

## Grants

| Access | Role | Actions | Resources |
|---|---|---|---|
| read | execution role | `ssm:GetParameters` | `…:parameter/<env>/<project>/vault/<name>/*` |
| read_write | task role, in addition | `ssm:PutParameter`, `ssm:GetParameter`, `ssm:GetParameters` | `…/vault/<name>/*` |
| read_write | task role, in addition | `ssm:GetParametersByPath` | `…/vault/<name>` and `…/vault/<name>/*` |

The execution role is what ECS uses to fetch a container's secrets when a task
starts. `read_write` is for an application that writes its own secrets, for
example a rotated token; there is no `ssm:DeleteParameter`. A secret the
application writes must still be declared in the YAML to reach a container.

Both grants are inline role policies, `<project>_vaults_read_<workload>_<env>`
and `<project>_vaults_write_<workload>_<env>`. The secrets the task definitions
list are ordered after the read grant, so a task definition that names a vault
secret is registered only once its execution role may read it.

## Moving from copied parameters

A project that copied the same parameter into several workloads' own paths can
move to a vault in one change:

1. Add the vault, its secret names and the connections to the YAML, and set
   the values in the Vaults tab.
2. Delete the copies under each workload's own path.
3. Apply, and deploy the connected workloads right after.

Between step 2 and that deploy, a new task of the old revision cannot start: the
old revision still names the deleted copies. Run steps 2 and 3 together.

Doing step 1 without step 2 is refused at plan: the same name would come from
the workload's own path and from the vault, and ECS refuses a container
definition with a name twice. If the copies must stay a little longer, connect
the vault with a prefix so the names differ.

## Name collisions: what is refused and when

ECS refuses to register a task definition that sets one name twice, or sets a
name both as a plain variable and as a secret, and it does so in the middle of
an apply. meroku refuses these earlier.

**At generate** (`app/vaults.go`), since the names are in the YAML:

- a secret named `env` on a connection without a prefix. meroku creates
  `<own path>/env` for every workload and sets it as `ENV`. Only the exact name
  `ENV` is reserved: with `prefix: PAY` it becomes `PAY_ENV`, which is allowed;
- two vaults connected to one workload giving the same variable (two vaults
  connected without prefixes that both declare `database_url`);
- a vault secret with the name of one of that workload's YAML variables
  (`workload.backend_env_variables`, `services[].env_vars`, a task's
  `environment_variables`);
- a vault secret with the name of a variable meroku sets for that workload
  (`DATA_BUCKET_*`, `EVENT_QUEUE_URL`, `VALKEY_*`, the backend's
  `AWS_S3_BUCKET`; `app/workload_env_reserved.go`).

**At plan**, in `modules/env_secret_check`, with two preconditions on every
task definition:

1. **Always at plan.** Built from the environment variables, `ENV` and the vault
   secrets, which are all known when the plan runs, on a new environment too.
   It repeats the generate checks and adds the variables the owning modules set
   themselves (`AWS_REGION`, `PG_DATABASE_HOST` and the rest), which only the
   modules list.
2. **Secrets under the workload's own path.** A parameter under
   `/<env>/<project>/<service>/` (or `…/backend/`, `…/task/<task>/`) with the name
   of a variable or of a vault secret. The message names both parameters.

   This check runs at plan whenever the own path is read at plan: always for
   scheduled and event tasks, and for the backend except on the plan that
   creates it.

### The one deferral: a plan that adds or removes a service

Every service's own-path read waits for the services' `/env` parameters, as one
collection, and that collection changes only when a service is added or
removed. On such a plan the second check, for **every** service, moves from plan
to apply. The first check still runs at plan.

Example: `api` has an own-path parameter `database_url`, a vault without a
prefix declaring `database_url` is connected to `api`, and a new service
`worker` is added in the same change. The plan succeeds. At apply, Terraform
reads the own path, the precondition fails before `api`'s task definition is
registered, and the error names both parameters. `api` keeps running its
current revision; whatever else the apply already did (the new service's
parameter, the grants) stays and the next apply completes it.

**Workaround: add the service and connect the vault in separate applies.** Apply
the new service first, then connect the vault; the second plan reads every own
path and refuses the collision at plan.

### Out-of-band parameters on a new workload

A parameter created by hand under a new backend's or service's own path, before
its first apply, is checked against the vaults and the environment at apply,
not at plan, for the same reason.

## Custom Terraform

A project's root-level custom files may read `module.vaults.vault_paths` and
`module.vaults.secrets`. A custom `.tf` that declares its own `module "vaults"`
is refused at generate; rename it.
