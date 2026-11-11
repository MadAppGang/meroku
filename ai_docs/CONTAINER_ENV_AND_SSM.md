# Container Environment Variables and SSM Secrets

How a meroku container gets its environment, why `ENV` is reserved, and what
to do when a plan fails with "would set X both as a plain environment variable
and as a secret".

## Two sources, one container

Every ECS container meroku defines (the backend, each service, each scheduled
task, each event processor task) gets variables from two places.

**Declared, in the env YAML.** These become the container's `environment`:

| Workload | YAML field |
|----------|------------|
| Backend | `workload.backend_env_variables` |
| Service | `services[].env_vars` |
| Scheduled task | `scheduled_tasks[].environment_variables` |
| Event processor task | `event_processor_tasks[].environment_variables` |

All four are **maps** of name to value:

```yaml
services:
  - name: orders
    env_vars:
      LOG_LEVEL: debug
      EVENT_BUS: local
```

A list of `- name: … / value: …` entries is not the format. meroku renders it
wrongly: the container gets variables literally called `name` and `value`.

meroku also adds its own variables, such as `AWS_REGION`, `URL`,
`PG_DATABASE_HOST`, `SERVICE_NAME` and `EVENT_SOURCE`. The lists are in
`modules/workloads/env.tf` (backend) and `modules/workloads/env_services.tf`
(services).

**Discovered, in SSM Parameter Store.** These become the container's
`secrets`. Every parameter under the workload's path is passed in, named after
the **upper-cased last segment** of the parameter name:

| Workload | SSM path |
|----------|----------|
| Backend | `/<env>/<project>/backend/` |
| Service | `/<env>/<project>/<service>/` |
| Scheduled or event processor task | `/<env>/<project>/task/<task>/` |

`/dev/myapp/orders/database_url` reaches the container as `DATABASE_URL`.

A parameter's value is read when a task starts. Changing it needs no Terraform
apply and no new task-definition revision. Restart the tasks (force a new
deployment) and they get the new value.

## `ENV` is reserved

For every workload, Terraform creates the parameter `<path>/env` (a
`SecureString`, value `" "`, with `ignore_changes` on the value). It exists so
the path is never empty, and it gives each workload one variable whose value is
set at runtime: `ENV`.

Because that parameter always exists, `ENV` is always a secret. Setting `ENV`
in the YAML as well puts the name in both `environment` and `secrets`, and ECS
refuses the task definition:

```
ClientException: The secret name must be unique and not shared with any new or
existing environment variables set on the container
```

Deleting the parameter does not help: Terraform creates it again on the next
apply.

To set `ENV`, leave it out of the YAML and write the parameter:

```bash
aws ssm put-parameter --profile <aws_profile> --region <region> \
  --name /<env>/<project>/<service>/env --value dev --overwrite
```

Then restart the tasks. A task started before the change keeps the old value.

## Checks

Two checks guard against a name in both lists.

1. **Generate time, Go.** `validateReservedEnvNamesConfigMap` in
   `app/env_reserved.go`, run from `preprocessEnvMap`, refuses `ENV` in any of
   the four YAML fields. The error names the workload and the parameter to set.
   It sees only the YAML, so it covers only the reserved name.
2. **Plan time, Terraform.** A precondition on each task definition compares
   the rendered `environment` names with the discovered secret names, using the
   `modules/env_secret_check` module (called from
   `modules/workloads/env_secret_check.tf`, `modules/ecs_task` and
   `modules/event_bridge_task`). It catches any collision, including one caused
   by a parameter someone created by hand, such as
   `/dev/myapp/orders/log_level` against `LOG_LEVEL` in the YAML. For an `ENV`
   collision the message says the name is reserved and gives the
   `put-parameter` command.

For any other name, keep exactly one of the two: delete the SSM parameter, or
remove the name from the YAML. If the name is one meroku sets itself, only
deleting the parameter is possible.

## Tests

- `go test ./app/ -run TestReservedEnv`
- `terraform -chdir=modules/env_secret_check test`
- `terraform -chdir=modules/workloads test -filter=tests/env_secret_collision.tftest.hcl`
