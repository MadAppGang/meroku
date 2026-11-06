# Backend container health check and deployment circuit breaker

Two optional keys under `workload:` for the backend ECS service. Both are absent by
default. An environment without them generates the same `main.tf` as before and plans
no change.

```yaml
workload:
  backend_health_check:
    command: "curl -fsS http://localhost:8080/health/live || exit 1"
    interval: 30       # optional, seconds, 5-300, default 30
    timeout: 5         # optional, seconds, 2-120, default 5
    retries: 3         # optional, 1-10, default 3
    start_period: 60   # optional, seconds, 0-300, default 60
  backend_circuit_breaker:
    enable: true       # optional, default true
    rollback: true     # optional, default true
```

## `backend_health_check`

The `healthCheck` of the backend container in the task definition. ECS runs the
command inside the container every `interval` seconds. After `retries` failures in a
row, ECS marks the container, and the task, unhealthy and replaces the task. Failures
during the first `start_period` seconds do not count.

`command` takes the same two forms as `services[].health_check_command`:

- A **string** is a shell command. meroku renders it as `["CMD-SHELL", "<string>"]`.
- A **list** is the raw ECS form and is passed through: `["CMD", "/app/healthcheck"]`
  for an image with no shell (distroless, scratch), or `["CMD-SHELL", "<one string>"]`.

The tool the command calls (`curl`, `wget`, your own binary) must exist in the image.
If it does not, every probe fails and ECS replaces the task in a loop. meroku never
invents a command.

`meroku generate` refuses a `backend_health_check` with no command, a list that does
not start with `CMD` or `CMD-SHELL`, and a number outside the ranges above. The
Terraform module checks the same rules.

The defaults match those of `services[].health_check_command`. The fields you leave
out are not written to `main.tf`; the module fills them in.

### What an apply does

The health check is part of the container definition. Adding, changing or removing
it makes Terraform register a new task definition revision. The revision
notification in `modules/workloads/backend.tf` then tells the CI Lambda, which deploys
the revision if `backend_auto_deploy` allows. Later image pushes clone the latest
revision, so they keep the health check.

### Cloud Map

The backend is registered in Cloud Map with `health_check_custom_config`. In that
mode, which is the only one ECS supports for a private DNS namespace, ECS reports
each task's health to Cloud Map. With a container health check, a task whose
container is unhealthy is reported unhealthy there too, until ECS replaces it.
Nothing needs to change in the Cloud Map service, and the tests check that it does
not.

The ALB target group health check, when `enable_alb` is on, is separate: it probes
`backend_health_endpoint` from the load balancer. A task has to pass both.

## `backend_circuit_breaker`

The `deployment_circuit_breaker` block of the backend `aws_ecs_service`. With
`enable: true`, ECS stops a deployment whose tasks keep failing to start or keep
failing the container health check. With `rollback: true` it also rolls the service
back to the last deployment that completed.

`backend_circuit_breaker: {}` turns both on.

**To turn the breaker off, set `enable: false`.** Removing the key changes nothing in
AWS. The AWS provider (6.x) hides the removal of the block from the plan, so the
service keeps the setting it was last applied with. The web app's "Roll back failed
deployments" switch writes `enable: false` for this reason.

## Where it lives

| Layer | File |
|-------|------|
| YAML type, round trip | `app/model.go` (`BackendHealthCheck`, `BackendCircuitBreaker`) |
| String → list, validation | `app/health_check_command.go`, called from `preprocessEnvMap` |
| Template | `env/main.hbs`, next to `backend_container_command` |
| Module | `modules/workloads/variables.tf`, `modules/workloads/backend.tf` |
| Web app | `web/src/components/BackendServiceProperties.tsx` |
| Tests | `app/backend_health_check_test.go`, `modules/workloads/tests/backend_health.tftest.hcl` |
