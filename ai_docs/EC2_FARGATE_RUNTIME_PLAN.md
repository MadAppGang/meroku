# Flexible EC2 + Fargate runtime for meroku

## Context

meroku is Fargate-only, and not by configuration — by literal. `launch_type = "FARGATE"`
appears as a hardcoded string in exactly five places, and `requires_compatibilities =
["FARGATE"]` in six. No `aws_autoscaling_group`, `aws_launch_template`,
`aws_ecs_capacity_provider`, or `aws_iam_instance_profile` exists anywhere in the 18
modules. The web UI presents Fargate as the product ("No EC2 instances to manage",
`ECSNodeProperties.tsx:457`), the Go API serves a hardcoded Fargate cpu/memory matrix
(`app/api_fargate.go:22-32`), and `app/pricing/types.go` has a `Fargate` field with no EC2
counterpart.

We want EC2 as a peer runtime for four reasons at once: cost at steady load, hardware
Fargate can't provide (GPU, high memory ratios, ARM), stateful needs (EBS, host mounts,
one-per-host daemons), and warm capacity with no ENI-attachment cold start. The choice must
be **per-service** — one cluster hosting both — and EC2 capacity must be driven by **ECS
managed scaling with an explicit min/max floor**, so the common case needs no thought and
the specific case stays reachable.

Intended outcome: `runtime: ec2` on a service is a one-line change that works with no other
config, and every knob (instance types, spot mix, AMI, volumes, user-data) is overridable
when a workload actually needs it.

---

## The shape: named compute pools

A **pool** is one launch template + one ASG + one ECS capacity provider. Services reference
a pool by name. This is the unit that answers "balanced default, override when needed".

```yaml
schema_version: 26

compute:
  pools:
    - name: general
      enabled: true
      # Everything below is optional. Omit the whole block and you get these values.
      instance_types: [m7i-flex.large, m6i.large, m6a.large]  # ASG picks by price+capacity
      capacity_type: spot_with_base      # on_demand | spot | spot_with_base
      on_demand_base: 1                  # keep N on-demand, rest spot
      min_size: 1                        # the warm floor
      max_size: 6                        # the cost ceiling
      target_capacity: 100               # ECS managed scaling target %

      # Escape hatches — none of these are needed for the default path
      ami_family: al2023                 # al2023 | al2023_arm64 | al2023_gpu | al2023_neuron
      ami_id: ""                         # explicit AMI, wins over ami_family
      root_volume_gb: 30
      extra_volumes: []                  # additional EBS devices
      user_data_extra: ""                 # appended after the ECS_CLUSTER config
      instance_policies: []              # extra IAM statements on the instance role

workload:
  backend_runtime: fargate               # fargate | fargate_spot | ec2
  backend_compute_pool: ""               # required only when runtime is ec2

services:
  - name: inference
    runtime: ec2
    compute_pool: gpu
```

**Zero-config path.** If any workload sets `runtime: ec2` and `compute.pools` is empty,
`applyTemplate` synthesizes a pool named `default` with the values above before rendering.
That is the "easiest way to create a set of EC2 instances": one field, no pool block.

**Why a list of instance types, not one.** A single instance type is the most common cause
of spot interruption and capacity failures. A three-type mixed-instances policy with
`price-capacity-optimized` allocation is strictly better and costs nothing to specify. The
default list is same-size, same-generation, three vendors (Intel flex, Intel, AMD).

---

## Terraform

### Where the pools live: inside `modules/workloads`

New file `modules/workloads/ec2_capacity.tf`, `for_each` over a new `var.compute_pools`.

Not a separate top-level module. The cluster is `aws_ecs_cluster.main` in
`modules/workloads/main.tf:1-12`, and a capacity provider must be attached to the cluster
(via `aws_ecs_cluster_capacity_providers`) **before** any service can reference it. Splitting
the pools into a sibling module invoked from `main.hbs` puts that ordering across a module
boundary, requiring hand-written `depends_on` that Terraform cannot infer. Keeping them in
the module that owns the cluster makes the dependency graph correct for free — and matches
how `services` already works (a list variable driving `for_each`, `services.tf:1-3`).

### Resources per pool

| Resource | Notes |
|---|---|
| `aws_launch_template.pool[name]` | AMI from SSM per `ami_family`; user-data writes `ECS_CLUSTER` + `ECS_ENABLE_HIGH_DENSITY_ENI=true`, then appends `user_data_extra`; IMDSv2 required |
| `aws_autoscaling_group.pool[name]` | `mixed_instances_policy` with `price-capacity-optimized`; **`protect_from_scale_in = true`** (mandatory when managed termination protection is on); subnets from `var.subnet_ids` |
| `aws_ecs_capacity_provider.pool[name]` | `managed_scaling { status = ENABLED, target_capacity, minimum_scaling_step_size, maximum_scaling_step_size }`, `managed_termination_protection = ENABLED`, `managed_draining = ENABLED` |
| `aws_ecs_cluster_capacity_providers.main` | **One** resource listing every pool provider plus `FARGATE` and `FARGATE_SPOT` |
| `aws_iam_role.ecs_instance` + `aws_iam_instance_profile.ecs_instance` | Shared across pools. `AmazonEC2ContainerServiceforEC2Role` + `AmazonSSMManagedInstanceCore` (so ECS Exec and Session Manager work without a bastion) |
| `aws_security_group.ecs_instance` | Shared. Egress all; no ingress needed under `awsvpc` |

`min_size`/`max_size` go on the ASG; ECS managed scaling moves `desired_capacity` between
them. That is exactly the "managed scaling + explicit floor" model: the floor is a warm-
capacity guarantee, the ceiling is a spend guard, and the number in between is ECS's problem.

### The launch_type → capacity_provider_strategy switch

The five hardcoded sites become conditional. Pattern, per service:

```hcl
# aws_ecs_service.backend
launch_type = local.backend_pool == null ? "FARGATE" : null

dynamic "capacity_provider_strategy" {
  for_each = local.backend_pool == null ? [] : [1]
  content {
    capacity_provider = aws_ecs_capacity_provider.pool[local.backend_pool].name
    weight            = 1
    base              = 0
  }
}
```

`launch_type` and `capacity_provider_strategy` are mutually exclusive in the AWS provider,
so this must be an either/or, never both.

Sites to change:

- `modules/workloads/backend.tf:16` (`launch_type`), `:21-25` (`network_configuration`),
  `:139-140` (`network_mode` / `requires_compatibilities`), `:142-143` and `:168-169`
  (the `max(cpu, 256)` / `max(memory, 512)` Fargate floors)
- `modules/workloads/services.tf:79`, `:85-89`, `:127-128`
- `modules/workloads/pgadmin.tf:65`, `:94` — phase 3
- `modules/ecs_task/main.tf:50`, `:89` — phase 3
- `modules/event_bridge_task/main.tf:52`, `ecs.tf:33` — phase 3

### Task definition differences on EC2

- `requires_compatibilities = ["EC2"]` instead of `["FARGATE"]`.
- **Drop the Fargate floors.** `max(var.backend_cpu, 256)` exists because Fargate rejects
  less. On EC2, task-level `cpu` is optional entirely and `memory` is a hard cap rather
  than a billed allocation. Keeping the floors would silently over-reserve.
- `assign_public_ip` in `network_configuration` is **Fargate-only** — must be conditional.
  Tasks still get public egress on EC2 because the pool's subnets have
  `map_public_ip_on_launch = true` (`modules/vpc/main.tf:47`); the instance carries the
  public IP, not the task ENI.
- Add `runtime_platform { cpu_architecture = "ARM64" }` when the pool's `ami_family` is
  `al2023_arm64`, otherwise Graviton pools will fail to place x86 images.

### Networking: keep `awsvpc`

Every ALB target group is `target_type = "ip"` (`alb.tf:69`, `services.tf:13`), every
service registers A+SRV records in Cloud Map (`backend.tf:40-44`, `services.tf:100-104`),
each task has its own security group, and container definitions set `hostPort ==
containerPort` (`backend.tf:197-202`) — all of which are `awsvpc` properties. Switching to
`bridge` mode with dynamic port mapping would mean changing target group types (a replace),
rewriting the Cloud Map registrations, and collapsing per-task security groups into
per-instance ones. That is a much larger blast radius than this change deserves.

**The cost of that choice, and it is real.** Under `awsvpc` on EC2, each task consumes an
ENI, and ENIs per instance are limited — an `m6i.large` gets 3 by default, so roughly 2
tasks per instance, which destroys the bin-packing that makes EC2 cheaper than Fargate in
the first place. The fix is ENI trunking: the account-level `awsvpcTrunking` setting plus
`ECS_ENABLE_HIGH_DENSITY_ENI=true` in instance user-data, which raises an `m6i.large` to
roughly 10+ tasks. The user-data half is ours. The account setting is not, so it becomes a
**preflight check** (see below) — without it, the cost motivation silently fails and looks
like a pricing bug.

---

## Go

### `app/model.go`

New types, plus fields on existing ones:

```go
type Compute struct {
    Pools []ComputePool `yaml:"pools,omitempty"`
}

type ComputePool struct {
    Name             string   `yaml:"name"`
    Enabled          *bool    `yaml:"enabled,omitempty"`
    InstanceTypes    []string `yaml:"instance_types,omitempty"`
    CapacityType     string   `yaml:"capacity_type,omitempty"`
    OnDemandBase     *int     `yaml:"on_demand_base,omitempty"`
    MinSize          *int     `yaml:"min_size,omitempty"`
    MaxSize          *int     `yaml:"max_size,omitempty"`
    TargetCapacity   *int     `yaml:"target_capacity,omitempty"`
    AMIFamily        string   `yaml:"ami_family,omitempty"`
    AMIID            string   `yaml:"ami_id,omitempty"`
    RootVolumeGB     *int     `yaml:"root_volume_gb,omitempty"`
    ExtraVolumes     []PoolVolume `yaml:"extra_volumes,omitempty"`
    UserDataExtra    string   `yaml:"user_data_extra,omitempty"`
    InstancePolicies []Policy `yaml:"instance_policies,omitempty"`
}
```

- `Env` (`model.go:12`) gains `Compute Compute \`yaml:"compute,omitempty"\``
- `Workload` (`model.go:168`) gains `BackendRuntime string` and `BackendComputePool string`
- `Service` (`model.go:393`) gains `Runtime string` and `ComputePool string`
- `createEnv` (`model.go:616`) sets `BackendRuntime: "fargate"` for new projects

Pointer types on the numeric pool fields deliberately: `min_size: 0` is meaningful (scale
to zero when idle) and must be distinguishable from absent. This is the same reason the
codebase uses `*bool` for `Enabled` and `AutoDeploy`.

Note the existing trap: `main.hbs:612-613` reads `workload.backend_autoscaling_target_cpu`
and `workload.efs`, neither of which is declared in the `Workload` struct — it works only
because rendering goes through `loadEnvToMap`, not `Env`. Do not repeat that here. These
fields must be on the struct because pricing, validation, and the TUI read the typed path.

### `app/migrations.go` — v26

Append to `AllMigrations` (line 55), bump `CurrentSchemaVersion` (line 39, currently 25),
add `migrateToV26` following the v7/v8 shape (guard with `if _, exists`, print
`  → Migrating to v26:`, never bump the version itself).

The migration writes `backend_runtime: fargate` on every `workload` and `runtime: fargate`
on every entry in `services`. **That is the safety property**: every existing environment
takes the `launch_type = "FARGATE"` branch and produces a byte-identical plan. Nothing moves
until someone edits a runtime field.

Remember nested maps come back as `map[interface{}]interface{}` from yaml.v2 — see the type
assertions at `migrations.go:234`, `:463`, `:1769`. Use `migrateV8ToV9` (`:456-521`) as the
model for iterating a collection.

### `app/validation.go` — refusals at generate time

New `validateComputeConfigMap(envMap)`, wired into `applyTemplate` alongside the ALB and
AppSync checks (`app/deploy.go:285-295`). Use `yamlSubMap` (`validation.go:455`) for map
access. Follow `validateALBConfigMap` (`:494-519`) for tone: name both the key and the fix.

Refuse:
- `runtime: ec2` naming a `compute_pool` that no pool defines
- pool with `min_size > max_size`
- pool with `capacity_type: on_demand` but `on_demand_base` set (contradiction)
- `ami_family: al2023_gpu` with no GPU instance type in `instance_types`
- `ami_family: al2023_arm64` with x86-only instance types
- an EC2 workload whose `memory` exceeds the largest instance in its pool — this is the
  failure that otherwise manifests as a task stuck in `PROVISIONING` forever with no error

Warn (don't refuse):
- `capacity_type: spot` with no on-demand base on a service that has an ALB target group
- a pool referenced by nothing

### `app/deploy.go`

Before `raymond.Parse` (around `deploy.go:308-313`, where `modules`/`has_custom_pre` are
injected): if any runtime is `ec2` and `compute.pools` is empty, inject the synthesized
`default` pool into `envMap`. Keeping this in the Go layer rather than in Handlebars means
the defaults are testable and appear in one place.

### `env/main.hbs`

Add to the `module "workloads"` block (`main.hbs:503-685`):

```handlebars
{{#compare (len compute.pools) ">" 0}}
  compute_pools = {{{array compute.pools}}}
{{/compare}}
  backend_runtime      = "{{default workload.backend_runtime "fargate"}}"
  backend_compute_pool = "{{default workload.backend_compute_pool ""}}"
```

`services` already passes through `{{{array services}}}` (`main.hbs:663`), so per-service
`runtime`/`compute_pool` need no template change — only `optional()` entries added to the
`services` object type in `modules/workloads/variables.tf:324-354`.

### Pricing

EC2 cost is fundamentally a different shape: instance-hours × instance count, not the sum of
per-task cpu/memory. A pool at `min_size: 1` costs money with zero tasks running. Touch:

- `app/pricing/types.go` — add `EC2Pricing{InstanceHourly map[string]float64; SpotDiscount float64}` and an `EC2` field on `PriceRates` (line 7-34)
- `app/pricing/aws_client.go:97` — fallback rates for the default instance types
- `app/pricing/calculators.go` — `CalculateEC2PoolPrice(pool, rates)`
- `app/api_pricing.go` — the four `calculate*Pricing` functions at `:256`, `:323`, `:722`, `:791` all hardcode `details["runtime"] = "Fargate"`; they need to branch, and an EC2-runtime service should report **$0 marginal** with the cost attributed to its pool, or the same instance gets billed once per task in the UI
- `web/src/utils/awsPricing.ts` and `web/src/services/pricingService.ts` mirror the Go formula and need the same `ec2` key

### New endpoint

`app/api_compute.go` serving `GET /api/compute/instance-families` — a curated instance list
with vCPU, memory, architecture, and GPU flag, so the UI dropdowns aren't a fourth hardcoded
table. Register in `mainRouter` (`app/spa_server.go:40-181`) as
`mux.HandleFunc("/api/compute/instance-families", corsMiddleware(getInstanceFamilies))`,
following `api_fargate.go`'s single-method-guard shape.

---

## Web UI

- `web/src/types/yamlConfig.ts` — add the `compute` block (`:18`), `workload.backend_runtime`
  / `backend_compute_pool` (`:46-105`), `services[].runtime` / `compute_pool` (`:304-340`).
  Hand-written, no codegen. Also update the second `Service` interface in
  `web/src/types/components.ts:42`.
- **Runtime toggle** — copy the Aurora-vs-RDS pattern at
  `PostgresNodeProperties.tsx:189-330`: a mode control plus two mutually exclusive
  `bg-gray-800 rounded-lg` panels gated on `{flag && …}` / `{!flag && …}`. It is the closest
  existing analogue and already handles cross-field auto-correction and inline `<Alert>`
  warnings.
- `BackendScalingConfiguration.tsx` — the real compute editor, dual-mode for backend and
  services. Two changes: the CPU/memory `Select`s must become free-form inputs (or use a
  different table) when runtime is `ec2`, and **the `useEffect` at `:116-126` that snaps
  memory to Fargate-legal values must be gated on runtime** or it will silently rewrite
  valid EC2 values as soon as the Fargate table loads.
- **Pool editing** — put it in the ECS cluster node's properties, replacing the hardcoded
  "Compute Configuration" bullet list at `ECSNodeProperties.tsx:450-462`. A new
  `compute-pool` node type would need four manual registrations (`types.ts:5`,
  `nodeStateMapping.ts:17`, `DeploymentCanvas.tsx:75`, and three separate places in
  `Sidebar.tsx`), which isn't worth it until pools prove they need their own panel.
- `nodeStateMapping.ts:71-80` hardcodes `launchType: "Fargate"` on the `ecs-cluster` node.
- Do **not** extend `ServiceAutoscaling.tsx` — it is dead code (only importer is
  `BackendAutoscaling.tsx`, which nothing imports), carries a stale duplicate of the
  cpu/memory matrix (`:57-70`, missing the 8192/16384 tiers), a third hardcoded pricing
  implementation (`:156-166`), and a Save button with no `onClick` (`:495`).

---

## Phasing

**Phase 0 — spike (blocks everything).** Verify on a scratch environment whether AWS
`UpdateService` accepts moving an existing service from `launch_type` to
`capacity_provider_strategy`. Historically it refused. If it refuses, switching an existing
service's runtime requires **replacing** the service, and service names collide so
`create_before_destroy` won't save it — which changes the migration story from "edit one
field" to "documented recreate with downtime". Everything downstream depends on this answer,
so it goes first and it is cheap to test.

**Phase 1 — YAML-only EC2 for backend + named services.** `modules/workloads/ec2_capacity.tf`,
the conditional launch_type/capacity_provider switch, `variables.tf` additions, `main.hbs`,
schema v26, validation, and tests. No UI. Shippable and useful on its own: a power user can
set `runtime: ec2` in YAML.

**Phase 2 — Web UI.** Runtime toggle, pool editor in the ECS node, relaxed cpu/memory,
instance-family endpoint, pricing.

**Phase 3 — the rest of the surface.** pgAdmin, scheduled tasks (`modules/ecs_task`),
event-driven tasks (`modules/event_bridge_task`), `scheduling_strategy = "DAEMON"` for
one-per-host containers (note: DAEMON forbids `desired_count` and autoscaling, so validation
must refuse the combination), host bind mounts, and per-pool EBS volumes.

---

## Landmines

1. **Capacity provider deletion wedges `terraform destroy`.** AWS refuses to delete a
   capacity provider still referenced by a service or by the cluster's provider list, and
   the error surfaces late in a destroy. Needs `lifecycle { create_before_destroy = true }`
   on the provider plus a documented destroy order. This is the single most likely source of
   "destroy hangs forever" reports.
2. **ENI trunking is an account setting we don't own.** Without it, EC2 bin-packing is ~2
   tasks per instance and the cost case evaporates. Add it to `app/aws_preflight.go`
   (`AWSPreflightCheck`, `:16-177`) as a check that runs when any pool exists.
3. **`protect_from_scale_in` and managed termination protection are coupled.** Enabling one
   without the other is an apply-time error.
4. **Pre-existing bug, in scope to notice, not necessarily to fix here:**
   `backend_autoscaling_target_cpu` and `_target_memory` are declared
   (`variables.tf:420`, `:426`) and passed (`main.hbs:612-613`) but never referenced — the
   policies hardcode 70.0 and 75.0 (`backend_autoscaling.tf:24`, `:43`). Don't let EC2 work
   paper over it.
5. **CI gates the new Terraform.** `.github/workflows/ci.yml` runs
   `terraform fmt -check -recursive -diff modules/workloads` and `terraform validate` in
   that directory. New files must pass both.

---

## Verification

**Terraform**
```bash
task ci:terraform          # fmt -check -recursive + init -backend=false + validate
```

**Go**
```bash
cd app && go test -race ./... -count=1
```

New tests, following existing shapes:

- **Template rendering** — use the shared `renderMainHBS(t, overlay)` helper at
  `app/autodeploy_template_test.go:32-77` and the `workloadsModuleBlock` extractor at `:80`.
  Table test asserting: default overlay renders `launch_type = "FARGATE"` and **no**
  `capacity_provider_strategy`; `backend_runtime: ec2` renders the strategy and **no**
  `launch_type`; a pool renders into `compute_pools`. `want: ""` means the argument must be
  absent, which is the convention in `scheduled_task_hardening_test.go:46`.
- **Migration** — `TestMigrateToV26`, `TestMigrationChain_IncludesV26`,
  `TestMigrateToV26_Idempotent`, `TestMigrateToV26_PreservesExistingValues`, following
  `app/migration_v25_test.go` as the template.
- **Validation** — a `{name, config, wantError}` table plus a second test asserting the
  error message names the offending keys, following `app/alb_validation_test.go:16-95`.

**Zero-diff proof (the important one).** After migrating an existing environment to v26:
```bash
task infra-gen-dev && task infra-plan env=dev   # must report no changes
```
If this shows any diff, the migration defaults are wrong.

**Real apply.** A scratch environment with one pool and one EC2 service: confirm the
instance registers with the cluster, the task places on it, ECS Exec reaches the container,
the ALB target becomes healthy, and scaling the service from 1→N grows the ASG. Then
`task infra-destroy` to prove the capacity provider tears down cleanly (landmine 1).
