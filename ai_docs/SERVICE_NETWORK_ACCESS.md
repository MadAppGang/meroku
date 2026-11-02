# Service network access

`services[].network_access` decides who may open a connection to a service's
container port. Added in schema v30.

```yaml
services:
  - name: web
    network_access: public      # default
  - name: magento-bridge
    network_access: internal
    # The image is oven/bun:*-slim, which has no wget or curl, so the probe
    # uses the runtime that is already there. See "Health check" below.
    health_check_command: ["CMD", "bun", "--eval", "await fetch('http://127.0.0.1:8080/health/live').then(r => process.exit(r.ok ? 0 : 1))"]
  - name: worker
    network_access: none
```

## The four values

| Value | Reachable from | ALB target group, listener rule, `load_balancer` | API Gateway route, integration, VPC Link | Security group ingress on the container port |
|---|---|---|---|---|
| `public` *(default)* | the project's front door only | if `enable_alb` | if `enable_alb` is off | the ALB's security group, or the service's own group (`self`, which is what the VPC Link carries) |
| `internal` | inside the VPC only | no | no | the VPC CIDR |
| `full_open` | front door, and the task's public IP directly | if `enable_alb` | always | `0.0.0.0/0` and `::/0` |
| `none` | nobody | no | no | none |

The decisions are made in three locals in `modules/workloads/services.tf`:
`service_on_alb`, `service_on_api_gateway` and `service_ingress_rules`.

**What no value changes:** the egress rule (all outbound open) and
`assign_public_ip`. With `egress_strategy: public_ip` there is no NAT gateway,
and the public IP is how a task pulls its image from ECR and reaches SSM,
CloudWatch Logs and anything else outside the VPC. Security groups are stateful,
so replies to the task's own outbound connections arrive without an ingress
rule, including under `none`.

## Why `internal` removes the API Gateway route

Before v30 every service had an API Gateway route (`ANY /<service>/{proxy+}`),
in ALB projects as well, and the `execute-api` URL is public. The route reaches
the task through a VPC Link whose network interfaces sit inside the VPC and
carry the service's own security group. A security group narrowed to the VPC
CIDR therefore still admits it. Without removing the route,
`https://<api-id>.execute-api.<region>.amazonaws.com/<service>/` would have
stayed open to the internet. The same reasoning applies to the ALB, whose nodes
are also inside the VPC.

## Why ingress is an attribute, not blocks

`ingress` on `aws_security_group` is optional and computed. A resource with
zero `ingress {}` blocks means "ingress not managed", so the rules already on
the group stay. Moving a `full_open` service to `none` would have planned no
change and left `0.0.0.0/0` open. The module writes `ingress = [...]` as an
attribute, where `[]` removes every rule. `tests/network_access.tftest.hcl`
asserts that `none` plans a known, empty set.

## Health check

ECS runs a container health check inside the container. Before v30 the only
health check was the ALB target group's (`/health/live`). An `internal` or
`none` service has no target group, so without `health_check_command` ECS
replaces a task only when its process exits. A task that is running but hung
stays in service.

`health_check_command` takes one of two forms:

```yaml
# A shell command. Runs through the container's /bin/sh.
health_check_command: "curl -f http://localhost:3000/health/live || exit 1"

# A program run directly, with no shell. The only option for images that have
# no /bin/sh (distroless, scratch).
health_check_command: ["CMD", "/app/healthcheck", "--port", "3000"]
```

The list is the AWS ECS form (`healthCheck.command` in the task definition,
copied from Docker's `HEALTHCHECK`): its first element is `CMD` (run the
program directly) or `CMD-SHELL` (run one string through the shell). meroku
turns the string form into `["CMD-SHELL", "<string>"]` before rendering
(`app/health_check_command.go`), so the Terraform module always receives a
list. A list that does not start with `CMD` or `CMD-SHELL` is refused at
generate time, naming the service; ECS would otherwise reject it partway
through the apply. The YAML keeps the form you wrote when meroku saves it.

### Check that the tool is in the image

Slim and distroless base images usually have no `wget` or `curl`:
`oven/bun:*-slim`, `node:*-slim`, `python:*-slim`, `gcr.io/distroless/*` and
`scratch` among them. Check before you write the command:

```bash
docker run --rm --entrypoint sh <image> -c 'command -v wget curl'
```

If neither is there, probe with the runtime the image already runs. Use the
list form, so no shell is needed:

```yaml
# Bun
health_check_command: ["CMD", "bun", "--eval", "await fetch('http://127.0.0.1:8080/health/live').then(r => process.exit(r.ok ? 0 : 1))"]
# Node 18+
health_check_command: ["CMD", "node", "-e", "fetch('http://127.0.0.1:3000/health/live').then(r => process.exit(r.ok ? 0 : 1), () => process.exit(1))"]
# Python
health_check_command: ["CMD", "python", "-c", "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8000/health/live', timeout=4).status == 200 else 1)"]
```

If the Dockerfile already has a `HEALTHCHECK`, copy its command: ECS ignores
the Dockerfile's `HEALTHCHECK`, but the command in it is known to work in that
image.

meroku never invents the command, because the tool it calls must exist in the
image: a check whose tool is missing fails on every probe, and ECS replaces the
task in a loop. `meroku` prints a warning at generate time for an
`internal` or `none` service without one. When set, it renders as
`healthCheck` with interval 30s, timeout 5s, 3 retries and a 60s start period.
When empty, no `healthCheck` key is rendered and the task definition is
unchanged.

`aws_ecs_service.services` ignores `task_definition` changes (CI owns the
running revision), so a new health check reaches the running service on the
next deploy, not on the `terraform apply` that registers it.

## Upgrading: the v30 migration

First bring the project's `infrastructure/` to the same version as the binary:
`meroku update-infra`. Older modules do not declare `network_access`, and
Terraform drops undeclared fields silently, so the plan would show no change.
Since v4.10.2 `meroku generate` refuses to run against an older
`infrastructure/`.

An absent value renders as `public`. Every service that existed before v30
behaved like `full_open`, so the v30 migration writes `network_access:
full_open` onto each existing service. An untouched environment plans no
change. The owner tightens a service by changing the value.

`full_open` is the pre-v30 ingress rule with every field stated
(`description = ""`, `prefix_list_ids = []`, `security_groups = []`,
`self = false`), which is what AWS reads back for a rule that never set them.
Confirm the first real environment you migrate plans 0 to change.

## What a change from `full_open` plans

Moving a service from `full_open` to `internal`:

- `aws_security_group.services["<name>"]`: updated in place (ingress rule
  replaced).
- `aws_lb_listener_rule.services["<name>"]` and
  `aws_lb_target_group.services["<name>"]`: destroyed. The listener rule goes
  first, because the target group cannot be deleted while a rule references it.
- `aws_ecs_service.services["<name>"]`: `load_balancer` removed. Under the
  pinned provider (`>= 5.34, < 6`) this is an in-place `UpdateService`, which
  triggers a rolling deployment. Check the plan says `update in-place` and not
  `must be replaced` before applying.
- `aws_apigatewayv2_route`, `_integration` and `_vpc_link` for the service:
  destroyed.

Listener rule priorities are computed over all service names, so removing one
service from the ALB leaves a gap and does not renumber the others.

## Limitations

- Services on an EC2 pool in `bridge` network mode have no task-level security
  group; their traffic is governed by the pool's instance security group. For
  them, `network_access` only controls the ALB and API Gateway wiring.
- The backend (`workload`) is not covered. It keeps its own security group
  (VPC CIDR plus the ALB).
- There is no field for extra CIDRs (an office IP, a partner network). If that
  is needed, add a separate optional list and keep `network_access` a single
  choice.
- Reaching an `internal` service from another VPC needs routing (peering,
  Transit Gateway, VPC Lattice) that meroku does not create.
