# Vaults

A vault is a set of secrets several workloads share. It is the SSM Parameter
Store path `/<env>/<project>/vault/<name>/`, and every parameter directly under
it reaches each connected workload as a secret, without copying parameters
between workloads' own paths and without custom Terraform. Built by
`modules/vaults`.

## YAML

```yaml
vaults:                                   # → /<env>/<project>/vault/<name>/
  - name: shared
  - name: payments

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
- A connection's `name` must be a vault of the top-level list, and a workload
  connects to a vault at most once.
- `access`: `read` (the default) or `read_write`.
- `prefix`: optional; upper-case letters, digits and underscores, starting with a
  letter. It goes before every secret name from that vault, joined with `_`.

`meroku generate` refuses anything else and names the field. There is no schema
migration: absent means no vaults.

The web UI has a **Vaults** node on the canvas, always shown, which adds and
removes vaults; removing one also removes every workload's connection to it.
Each workload panel (backend, service, scheduled task, event task) has a
**Vaults** tab to connect a vault, pick read or read-write and set a prefix.

## Secret names

A parameter `/dev/myapp/vault/payments/database_url` becomes the secret

- `DATABASE_URL` for a workload connected without a prefix;
- `PAY_DATABASE_URL` for one connected with `prefix: PAY`.

The name is the last path segment, upper-cased, behind the prefix. Only
parameters directly under the vault count: the read is not recursive, so
`/dev/myapp/vault/payments/old/database_url` is not a secret of anyone.

Secrets are sorted by name, so the order of the YAML or of the SSM listing never
changes a task definition.

## Grants

| Access | Role | Actions | Resources |
|---|---|---|---|
| read | execution role | `ssm:GetParameters` | `…:parameter/<env>/<project>/vault/<name>/*` |
| read_write | task role, in addition | `ssm:PutParameter`, `ssm:GetParameter`, `ssm:GetParameters` | `…/vault/<name>/*` |
| read_write | task role, in addition | `ssm:GetParametersByPath` | `…/vault/<name>` and `…/vault/<name>/*` |

The execution role is what ECS uses to fetch a container's secrets when a task
starts. `read_write` is for an application that writes its own secrets, for
example a rotated token; there is no `ssm:DeleteParameter`. A deleted parameter
is still listed in every connected workload's running task definition, and every
new task of every one of those workloads would fail to start until the next
apply removes it. Delete a vault parameter with the AWS CLI and apply right
after.

Both grants are inline role policies, `<project>_vaults_read_<workload>_<env>`
and `<project>_vaults_write_<workload>_<env>`. The secrets the task definitions
list are ordered after the read grant, so a task definition that names a vault
secret is registered only once its execution role may read it.

## Setting a secret

meroku never stores the values.

```sh
aws ssm put-parameter --type SecureString \
  --name /dev/myapp/vault/payments/database_url \
  --value '<value>'
```

then run `meroku` apply. The vault is read at plan, so the plan shows a new
revision of every connected task definition with the new secret.

### Changing a value needs a redeploy

Adding or removing a parameter changes the connected task definitions on the
next apply. Changing a value in place (`put-parameter --overwrite`) does not:
the task definition names the parameter, not its value, and meroku's
deployment Lambda only reacts to parameter changes under a workload's own path.
A running task keeps the value it started with. Redeploy each connected workload
(or let its tasks be replaced) to pick up a new value.

## Moving from copied parameters

A project that copied the same parameter into several workloads' own paths can
move to a vault in one change:

1. Create the vault's parameters (`put-parameter` under `/…/vault/<name>/`).
2. Add the vault and the connections to the YAML.
3. Delete the copies under each workload's own path.
4. Apply, and deploy the connected workloads right after.

Between step 3 and that deploy, a new task of the old revision cannot start: the
old revision still names the deleted copies. Run steps 3 and 4 together.

Doing step 2 without step 3 is refused at plan: the same name would come from
the workload's own path and from the vault, and ECS refuses a container
definition with a name twice. If the copies must stay a little longer, connect
the vault with a prefix so the names differ.

## Name collisions: what is refused and when

ECS refuses to register a task definition that sets one name twice, or sets a
name both as a plain variable and as a secret, and it does so in the middle of
an apply. meroku checks every task definition first, in
`modules/env_secret_check`, with two preconditions:

1. **Always at plan.** Built from the environment variables, `ENV` and the vault
   secrets, which are all known when the plan runs, on a new environment too:
   - a variable meroku sets for the workload (the data bucket's, for example)
     appears again in the environment, from the YAML;
   - a vault secret has the name of an environment variable;
   - two vault secrets have the same name (two vaults connected without
     prefixes that both hold `database_url`);
   - a vault secret is named `ENV`. meroku creates `<own path>/env` for every
     workload and sets it as `ENV`, so a vault parameter named `env` connected
     without a prefix is refused, every time, at plan. Only the exact name `ENV`
     is reserved: with `prefix: PAY` it becomes `PAY_ENV`, which is allowed.
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
prefix exposing `DATABASE_URL` is connected to `api`, and a new service `worker`
is added in the same change. The plan succeeds. At apply, Terraform reads the
own path, the precondition fails before `api`'s task definition is registered,
and the error names both parameters. `api` keeps running its current revision;
whatever else the apply already did (the new service's parameter, the grants)
stays and the next apply completes it.

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
