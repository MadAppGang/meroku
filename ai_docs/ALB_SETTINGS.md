# ALB settings

Four optional keys tune the ALB ingress path. Every one defaults to what meroku has
always done. An environment that sets none of them generates the same `main.tf` as
before and plans no change. None of them needs a schema migration.

```yaml
alb:
  enabled: true
  ssl_policy: ELBSecurityPolicy-TLS13-1-2-2021-06   # default ELBSecurityPolicy-2016-08
  deregistration_delay: 30                          # seconds, 0-3600, default 300
  remove_api_gateway: true                          # default false

workload:
  backend_health_check_grace_period: 120            # seconds, default 0 (not set)
```

Absent keys render nothing into `env/<env>/main.tf`. The defaults live once, in the
`modules/workloads` variables. An explicit `0` or `false` is rendered as given.

| YAML key | Module variable | Default | What it sets |
|---|---|---|---|
| `alb.ssl_policy` | `alb_ssl_policy` | `ELBSecurityPolicy-2016-08` | `ssl_policy` of the HTTPS listener (`aws_lb_listener.https`) |
| `alb.deregistration_delay` | `alb_deregistration_delay` | `300` | `deregistration_delay` of every ALB target group: backend, backend_instance, services, services_instance |
| `workload.backend_health_check_grace_period` | `backend_health_check_grace_period` | `0` | `health_check_grace_period_seconds` of `aws_ecs_service.backend`, only while the ALB is on and the value is above 0; otherwise null |
| `alb.remove_api_gateway` | `remove_api_gateway` | `false` | With the ALB on, the API Gateway resources below are not created |

All four change existing resources in place. None replaces a resource.

## `ssl_policy`

An ELB security policy name, for example `ELBSecurityPolicy-TLS13-1-2-2021-06`.
`meroku generate` refuses anything that is not letters, digits and dashes. AWS checks
that the name exists at apply time.

## `deregistration_delay`

How long a target group keeps sending in-flight requests to a target that is being
deregistered, in seconds, 0 to 3600. A lower value makes rolling deploys finish faster;
a value lower than your longest request cuts that request off.

## `backend_health_check_grace_period`

How long ECS ignores failing ALB health checks on a newly started backend task. Set it
when the backend takes longer to become healthy than the target group's health check
allows, or ECS stops the task before it is ready.

**Set it in the same change that turns the ALB on.** Adding the load balancer to the
backend service starts a new deployment, and a slow-starting backend with no grace
period can be killed by the first failing health checks.

With the ALB off there is no load-balancer health check, so the value is ignored and
not refused. It can be set before `alb.enabled` is turned on.

## `remove_api_gateway`

With `alb.enabled: true` and `remove_api_gateway: true`, these are not created:

- `aws_apigatewayv2_api.api_gateway` (the execute-api URL)
- `aws_apigatewayv2_stage.backend`
- `aws_cloudwatch_log_group.api_gateway_logs` (API Gateway access logs)
- `aws_apigatewayv2_vpc_link.backend`
- every service's API Gateway VPC Link, integration and route

The backend's Cloud Map service, the backend service's `service_registries` and the
backend security group are not touched.

### Refused at `meroku generate`

| Situation | Why | Fix |
|---|---|---|
| `alb.enabled` is not true | With the ALB off, API Gateway is the only way in | Turn the ALB on, or remove the key |
| A service has `network_access: full_open` | full_open always has an API Gateway route | Set the service to `network_access: public`, which serves it from the ALB |
| An `extensions.sns_topics[]` entry has `webhooks` | The webhooks are delivered to the API Gateway URL | Remove the webhooks, or keep the gateway |

Disabled services (`enabled: false`) are not checked. The rules live in
`app/alb_settings.go` (`validateALBSettingsConfigMap`), called from `preprocessEnvMap`.

Every service that existed before schema v30 was pinned to `full_open` by the v30
migration, so most older environments must move their services to `public` first.
That change narrows each service's security group from the internet to the ALB's
security group. Apply it on its own, before turning on `remove_api_gateway`.

### What becomes empty

With the gateway removed:

- the `api_gateway_endpoint` and `api_gateway_id` outputs of `modules/workloads` are `""`;
- so are the env outputs built from them, and `api_gateway_id` in the bridge passed to
  custom modules (`app/deploy.go`).

Anything that used the execute-api URL or the API id must be moved to `api_domain`
first: an external client, a custom module, or a webhook configured outside meroku.

### It is destructive and not cleanly reversible

Turning `remove_api_gateway` on deletes the execute-api URL, the stage and the access
log group with its logs. Deleting a VPC Link can take several minutes.

Turning it back off creates a **new** API with a **new** id and execute-api URL. The
old URL does not come back.

## The api_domain record during the switch

`api_domain` is served by one Route 53 record, `aws_route53_record.backend` in
`modules/workloads/domain.tf`, in both ingress modes. Only its alias target differs:
API Gateway's custom domain with the ALB off, the ALB with it on. Flipping
`alb.enabled` is therefore one in-place UPSERT of that record, and the name keeps
resolving throughout.

Before this, each mode had its own record (`aws_route53_record.alb_alias` in `alb.tf`
for the ALB), and the switch deleted one record and created the other. The name did
not resolve between the two calls.

During the switch from API Gateway to the ALB, Terraform may delete the API Gateway
custom domain before it updates the record. For those seconds the record still
exists but points at a domain that no longer answers. This is a short window of
errors, not a name that fails to resolve.

## Address changes and `moved` blocks

These resources gained `count`, and `moved` blocks keep every environment's existing
objects, so upgrading meroku plans no change:

| From | To |
|---|---|
| `aws_route53_record.alb_alias` | `aws_route53_record.backend` |
| `aws_apigatewayv2_api.api_gateway` | `aws_apigatewayv2_api.api_gateway[0]` |
| `aws_apigatewayv2_vpc_link.backend` | `aws_apigatewayv2_vpc_link.backend[0]` |
| `aws_cloudwatch_log_group.api_gateway_logs` | `aws_cloudwatch_log_group.api_gateway_logs[0]` |
| `aws_apigatewayv2_stage.backend` | `aws_apigatewayv2_stage.backend[0]` |

The blocks must stay for as long as any environment may still hold the old address.
A plan after upgrading shows "has moved to" for each, and no change.
