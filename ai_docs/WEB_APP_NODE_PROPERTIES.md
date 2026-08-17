# Web App Node & Property Model

Every node the canvas can render, every property it carries, the data type, the conditionals that
switch it on or off, and its mutability class.

---

## Mutability classes

| Code | Meaning |
|---|---|
| **CREATE** | Set once. Changing it does not migrate anything — it defines a different stack, or forces a destroy/create with data loss. |
| **RO** | Read-only. Assigned by AWS, never settable. ARNs, endpoints, IDs, generated names, versions. |
| **TF** | Editable. Takes effect only after a Terraform redeploy. Updated in place. |
| **TF-REPLACE** | Editable. Needs a Terraform redeploy, and the resource is destroyed and recreated. Downtime, and for stateful resources, data loss. |
| **TF-DEPLOY** | Editable. Needs a Terraform redeploy that produces a new task definition revision and a rolling restart. No data loss. |
| **LIVE** | Editable at runtime. Applied to AWS immediately, no Terraform. |
| **LIVE-RESTART** | Editable at runtime, no Terraform, but the running container only reads it at start — needs a task restart to observe. |
| **RUNTIME** | Pure telemetry. Read-only and self-changing: CPU load, task counts, health, status. |
| **UI** | Never reaches AWS. Canvas layout and editor state only. |

### The add/remove vs. change distinction

Some properties are live to *change* but need a Terraform redeploy to *add or remove*, because the
task definition holds a reference list, not the value itself.

| Operation | Terraform? | Restart? |
|---|---|---|
| Change a parameter's value | no | yes |
| Change a parameter's description | no | no |
| Add a parameter | yes | yes |
| Remove a parameter | yes | yes |
| Change a parameter's type | not supported — delete + recreate | yes |

The same asymmetry applies to S3 environment files: writing the file's contents is live, adding or
removing a file from the reference list is a redeploy.

---

## Node inventory

| # | Node | Editable | Instantiated |
|---|---|---|---|
| 1 | `client-app` | no | fixed |
| 2 | `github` | yes | fixed |
| 3 | `api-gateway` | read-only | fixed |
| 4 | `alb` | yes | fixed |
| 5 | `route53` | yes | fixed |
| 6 | `ecs` | yes | fixed |
| 7 | `backend` | yes | fixed |
| 8 | `service` | yes | one per service |
| 9 | `ecr` | yes | fixed |
| 10 | `postgres` / `aurora` | yes | fixed |
| 11 | `s3` | yes | fixed |
| 12 | `eventbridge` | action-only | fixed |
| 13 | `event-task` | yes | one per event task |
| 14 | `scheduled-task` | yes | one per scheduled task |
| 15 | `sns` | yes | fixed |
| 16 | `sqs` | yes | conditional on SQS enabled |
| 17 | `ses` | yes | fixed |
| 18 | `cloudwatch` | read-only | fixed |
| 19 | `xray` | yes | fixed |
| 20 | `secrets-manager` | yes (live) | fixed |
| 21 | `efs` | yes | conditional on EFS volumes |
| 22 | `appsync` | yes | conditional on AppSync enabled |
| 23 | `amplify` | yes | one per Amplify app |
| 24 | `cloudfront` | yes | one per distribution |
| 25 | `custom-terraform` | yes | fixed |
| 26 | `alarms` | disabled | fixed |

Plus two layout-only kinds: `group` and `dynamicGroup`.

---

## Environment-level properties

Shown on the `ecs` node; they scope the whole stack.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `project` | string | required | CREATE | Prefix on every resource name. Changing it orphans the existing stack. |
| `env` | string | required | CREATE | Suffix on every resource name. Same blast radius. |
| `is_prod` | boolean | required | TF | Gates production-only behaviour and DNS root logic. |
| `region` | string | required | CREATE | A change is a full rebuild in a new region. |
| `account_id` | string | optional | RO | Derived from the active AWS profile. |
| `state_bucket` | string | required | CREATE | Changing it detaches state; needs manual state migration. |
| `state_file` | string | required | CREATE | Same. |
| `schema_version` | int | automatic | RO | Managed by migrations. Current: 24. |
| `use_default_vpc` | boolean | default `false` | TF-REPLACE | Flipping recreates every subnet-bound resource. |
| `vpc_cidr` | string | only when `use_default_vpc` is false; default `10.0.0.0/16` | TF-REPLACE | Replaces the VPC and everything in it. |
| `api_domain` | string | optional | TF | Custom domain for API Gateway. |

Runtime values on the same node:

| Property | Type | Class |
|---|---|---|
| `clusterName` | string | RO |
| `clusterArn` | string | RO |
| `status` | string | RUNTIME |
| `registeredTasks` | int | RUNTIME |
| `runningTasks` | int | RUNTIME |
| `activeServices` | int | RUNTIME |
| `capacityProviders` | string[] | RO |
| `containerInsights` | string | RO |
| `vpc.vpcId` | string | RO |
| `vpc.cidrBlock` | string | RO |
| `vpc.state` | string | RO |
| `availabilityZones` | string[] | RO |
| `subnets[].subnetId` | string | RO |
| `subnets[].availabilityZone` | string | RO |
| `subnets[].cidrBlock` | string | RO |
| `subnets[].type` | string | RO |
| `subnets[].availableIpCount` | int | RUNTIME |
| `serviceDiscovery.namespaceId` | string | RO |
| `serviceDiscovery.namespaceName` | string | RO |
| `serviceDiscovery.serviceCount` | int | RUNTIME |

---

## 1. `client-app`

Representational only. Not an AWS resource.

| Property | Type | Conditional | Class |
|---|---|---|---|
| `id` | string | — | UI |
| `name` | string | — | UI |
| `description` | string | — | UI |
| `position.x` | number | — | UI |
| `position.y` | number | — | UI |

---

## 2. `github`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.enable_github_oidc` | boolean | default `false` | TF | Creates the OIDC provider and the CI/CD IAM role. |
| `workload.github_oidc_subjects` | string[] | only when OIDC enabled | TF | Trust-policy `sub` claims. Editing rewrites the trust policy. |
| IAM role ARN | string | — | RO | |
| ECR registry URL | string | derived from ECR strategy | RO | Points at the source account under cross-account. |
| OAuth device session | object | — | RUNTIME | Session-scoped, never persisted. |

---

## 3. `api-gateway`

Read-only panel. Values are derived.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `api_domain` | string | optional | TF | Edited on the `route53` node. |
| Protocol | `"HTTP"` | fixed | RO | HTTP API v2. |
| Stage | string | fixed `$default` | RO | |
| Endpoint URL | string | — | RO | AWS-assigned. |
| API ID | string | — | RO | |
| Rate limit | int | — | TF | Module-level, not per-environment. |
| Burst limit | int | — | TF | Module-level. |
| Log retention | int (days) | — | TF | 30 days project-wide. |
| Backend route | string | derived from services | TF | Changes when the service list changes. |
| Auto-deploy | boolean | fixed `true` | RO | |

---

## 4. `alb`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `alb.enabled` | boolean | required when present | TF | Switches ingress from API Gateway to ALB. Both are created; this selects the routing path. |
| `workload.backend_alb_domain_name` | string | meaningful only when ALB enabled | TF | Host-header listener rule plus certificate and DNS record. |
| `workload.backend_health_endpoint` | string | — | TF | Target group health check path. |
| `workload.backend_image_port` | number | — | TF-DEPLOY | Target group port; also rewrites the task definition. |
| Routing rules | object[] | derived from public services | TF | Priority-ordered, derived, not free-form. |
| ALB DNS name | string | — | RO | |
| ALB ARN | string | — | RO | |
| Target group ARNs | string | — | RO | |
| Target health | string | — | RUNTIME | healthy / unhealthy / draining. |
| Listener certificate ARN | string | only with a configured domain | RO | |

---

## 5. `route53`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `domain.enabled` | boolean | required | TF | Master switch for DNS and certificates. |
| `domain.domain_name` | string | required when enabled | TF-REPLACE | Always the root domain. Changing it replaces the zone and every certificate. |
| `domain.create_domain_zone` | boolean | only when enabled | TF-REPLACE | `true` creates the zone, `false` adopts an existing one. Flipping destroys or orphans it. |
| `domain.zone_id` | string | required when not creating the zone | TF / RO | Read-only when meroku created the zone; editable when adopting one. |
| `domain.api_domain_prefix` | string | only when enabled | TF | e.g. `api`. |
| `domain.add_env_domain_prefix` | boolean | only when enabled | TF | Inserts the environment name for non-prod. |
| `domain.root_zone_id` | string | delegated subdomain environments | RO | Written by the DNS setup wizard. |
| `domain.root_account_id` | string | delegated subdomain environments | RO | |
| `domain.is_dns_root` | boolean | — | CREATE | Marks the account owning the apex zone. |
| `domain.dns_root_account_id` | string | — | RO | |
| `domain.delegation_role_arn` | string | cross-account delegation | RO | Assumed role for NS-record writes. |
| `domain.additional_domains[].domain` | string | — | TF-REPLACE | Array key. |
| `domain.additional_domains[].create_zone` | boolean | — | TF-REPLACE | |
| `domain.additional_domains[].zone_id` | string | only when not creating | TF | |
| `domain.additional_domains[].create_certificate` | boolean | default `true` | TF | |
| Zone NS records | string[] | — | RO | Assigned at zone creation. |
| Record set list | object[] | — | RUNTIME | |
| Delegation propagation status | string | — | RUNTIME | Resolver-observed, not stored. |

---

## 6. `ecs`

Carries the environment-level properties above, plus:

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.slack_webhook` | string | optional | TF | Alarm notification target. Stored in plaintext — treat as a secret. |
| `workload.setup_fcnsns` | boolean | optional | TF | Also surfaced on the `sns` node. |

---

## 7. `backend`

The largest property surface in the app.

### Container and image

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.backend_external_docker_image` | string | optional | TF-DEPLOY | When set, bypasses the managed registry. |
| `workload.backend_container_command` | string[] | optional | TF-DEPLOY | Entrypoint override. |
| `workload.backend_image_port` | number | default `8080` | TF-DEPLOY | Container port and target group port. |
| `workload.backend_health_endpoint` | string | default `/health` | TF-DEPLOY | |
| Deployed image tag | string | — | RUNTIME | Task definition changes are ignored on redeploy, so CI/CD pushes are not reverted. |

### Compute and scaling

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.backend_cpu` | string | Fargate-valid pairs only | TF-DEPLOY | `"256"` … `"16384"`. String, not number. |
| `workload.backend_memory` | string | must pair with CPU | TF-DEPLOY | |
| `workload.backend_desired_count` | number | default `1` | TF | Not ignored on redeploy. If autoscaling moved the count, the next redeploy resets it to this value. |
| `workload.backend_autoscaling_enabled` | boolean | default `false` | TF | Creates the scaling target and policies. |
| `workload.backend_autoscaling_min_capacity` | number | only when autoscaling enabled | TF | |
| `workload.backend_autoscaling_max_capacity` | number | only when autoscaling enabled | TF | |
| `runningCount` | int | — | RUNTIME | |
| `pendingCount` | int | — | RUNTIME | |
| `currentCPUUtilization` | number (%) | — | RUNTIME | |
| `currentMemoryUtilization` | number (%) | — | RUNTIME | |
| `targetCPU` | number (%) | only when autoscaling enabled | RO | Read back from the deployed policy. |
| `targetMemory` | number (%) | only when autoscaling enabled | RO | |
| `lastScalingActivity` | object | — | RUNTIME | |
| Scaling history events | object[] | — | RUNTIME | |

### Networking and access

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.backend_remote_access` | boolean | default `false` | TF-DEPLOY | Enables exec into the container. |
| `workload.backend_alb_domain_name` | string | only when ALB enabled | TF | |
| `domain.api_domain_prefix` | string | only when domain enabled | TF | |
| SSH capability | boolean | derived from remote access and a running task | RUNTIME | |
| Exec session | stream | — | RUNTIME | Not persisted. |

### Environment and secrets

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.backend_env_variables` | Record\<string,string\> | optional | TF-DEPLOY | Plaintext in the task definition. Not for secrets. |
| `workload.env_files_s3[].bucket` | string | — | TF-DEPLOY | Adding or removing an entry changes the task definition. |
| `workload.env_files_s3[].key` | string | — | TF-DEPLOY | |
| Environment file **contents** | string | — | LIVE-RESTART | Written directly. Needs a task restart. |
| Parameter **value** | string | — | LIVE-RESTART | Written directly. Needs a task restart. |
| Parameter **add / remove** | — | — | TF-DEPLOY | Changes the reference list in the task definition. |
| Parameter `type` | `"String"` \| `"StringList"` \| `"SecureString"` | — | LIVE, create-only | Cannot be changed on an existing parameter. |
| Parameter `version` | int | — | RO | Incremented on each write. |
| Parameter `arn` | string | — | RO | |
| Parameter `lastModifiedDate` | string (ISO) | — | RO | |

### Storage

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.bucket_postfix` | string | optional | TF-REPLACE | Part of the bucket name. Bucket names are immutable — a change destroys and recreates, losing objects. |
| `workload.bucket_public` | boolean | default `false` | TF | Public access block and bucket policy. |
| `workload.efs[].name` | string | must match a defined volume | TF-DEPLOY | |
| `workload.efs[].mount_point` | string | — | TF-DEPLOY | |
| Bucket ARN | string | — | RO | |
| Bucket regional domain | string | — | RO | |

### Observability

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.xray_enabled` | boolean | default `false` | TF-DEPLOY | Adds the tracing sidecar. |
| Log group name | string | — | RO | |
| Log retention | int (days) | fixed `30` | TF | Module-level. |
| Log stream contents | object[] | — | RUNTIME | |
| Metrics | object | — | RUNTIME | |
| Alarm definitions | object[] | — | TF | |

### IAM

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.policy[].actions` | string[] | optional | TF | Inline statements on the task role. |
| `workload.policy[].resources` | string[] | optional | TF | |
| Task role ARN | string | — | RO | |
| Execution role ARN | string | — | RO | |

### CI/CD, database admin, notifications

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.backend_auto_deploy` | boolean | absent means `true` | TF | Whether a new image may redeploy without asking. Manual deploy always works. |
| `workload.install_pg_admin` | boolean | requires Postgres enabled | TF | Creates the pgAdmin service. |
| `workload.pg_admin_email` | string | only when pgAdmin installed | TF-DEPLOY | Initial login. |
| `workload.setup_fcnsns` | boolean | optional | TF | |
| `workload.slack_webhook` | string | optional | TF | |

---

## 8. `service`

One node per additional service.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `name` | string | required | CREATE / TF-REPLACE | Identity key. Renaming destroys the service, registry, log group, target group and security groups, then creates new ones. |
| `enabled` | boolean | default `true` | TF-REPLACE | `false` removes the service from AWS entirely. |
| `auto_deploy` | boolean | absent means `true` | TF | CI/CD policy only. Distinct from `enabled`. |
| `docker_image` | string | optional | TF-DEPLOY | External image; otherwise the managed registry. |
| `container_command` | string[] | optional | TF-DEPLOY | |
| `container_port` | number | — | TF-DEPLOY | |
| `host_port` | number | optional | TF-DEPLOY | Must equal `container_port` under awsvpc networking. |
| `cpu` | number | Fargate-valid pairs | TF-DEPLOY | Number here, unlike the backend's string. |
| `memory` | number | must pair with CPU | TF-DEPLOY | |
| `desired_count` | number | — | TF | Same autoscaling-drift caveat as the backend. |
| `remote_access` | boolean | default `false` | TF-DEPLOY | |
| `xray_enabled` | boolean | default `false` | TF-DEPLOY | |
| `essential` | boolean | default `true` | TF-DEPLOY | Container-level essential flag. |
| `public_access` | boolean | default `false` | TF | Adds an ingress route and security group rule. |
| `health_check_path` | string | meaningful only when public | TF | |
| `api_domain_prefix` | string | requires domain enabled | TF | Per-service subdomain. |
| `env_vars` | Record\<string,string\> | legacy alias | TF-DEPLOY | |
| `environment_variables` | Record\<string,string\> | preferred | TF-DEPLOY | |
| `env_variables[].name` | string | legacy array form | TF-DEPLOY | |
| `env_variables[].value` | string | legacy array form | TF-DEPLOY | |
| `env_files_s3[].bucket` | string | optional | TF-DEPLOY | Contents are LIVE-RESTART. |
| `env_files_s3[].key` | string | optional | TF-DEPLOY | |
| `ecr_config.mode` | `"create_ecr"` \| `"manual_repo"` \| `"use_existing"` | — | TF-REPLACE | Switching away from `create_ecr` destroys the managed repo and its images. |
| `ecr_config.repository_uri` | string | only when `manual_repo` | TF-DEPLOY | |
| `ecr_config.source_service_name` | string | only when `use_existing` | TF-DEPLOY | |
| `ecr_config.source_service_type` | `"services"` \| `"event_processor_tasks"` \| `"scheduled_tasks"` | only when `use_existing` | TF-DEPLOY | |
| Parameter values | string | — | LIVE-RESTART | Add/remove is TF-DEPLOY. |
| ECS service name | string | — | RO | |
| Task definition ARN | string | — | RO | |
| `runningCount` | int | — | RUNTIME | |
| `pendingCount` | int | — | RUNTIME | |
| `status` | string | — | RUNTIME | |
| `launchType` | string | fixed `FARGATE` | RO | |

Three overlapping environment-variable shapes exist for backward compatibility: `env_vars`,
`environment_variables` and `env_variables[]`. Prefer `environment_variables`. All three land in the
task definition in plaintext.

---

## 9. `ecr`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `ecr_strategy` | `"local"` \| `"cross_account"` | default `local` | TF-REPLACE | Switching from `local` destroys the managed repositories and their images. |
| `ecr_account_id` | string | required when cross-account | TF-DEPLOY | Rewrites every image URI in the task definitions. |
| `ecr_account_region` | string | required when cross-account | TF-DEPLOY | |
| `ecr_trusted_accounts[].account_id` | string | only when strategy is `local` | TF | Grants pull-only access. In place. |
| `ecr_trusted_accounts[].env` | string | — | TF | Bookkeeping for the UI dropdown. |
| `ecr_trusted_accounts[].region` | string | — | TF | |
| Repository URI | string | — | RO | |
| Repository ARN | string | — | RO | |
| Image tag list | object[] | — | RUNTIME | |
| Image pushed-at | string | — | RUNTIME | |
| Image size | int | — | RUNTIME | |
| Trust policy deployment status | boolean | — | RUNTIME | Compares intent against the deployed policy. Yellow means configured but not yet deployed. |

---

## 10. `postgres` / `aurora`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `postgres.enabled` | boolean | required | TF-REPLACE | `false` destroys the database. |
| `postgres.aurora` | boolean | default `false` | TF-REPLACE | RDS instance and Aurora cluster are different resources. Flipping destroys the data. Snapshot first. |
| `postgres.dbname` | string | defaults to project name | TF-REPLACE | AWS forces replacement. |
| `postgres.username` | string | defaults to `postgres` | TF-REPLACE | Master username change forces replacement. |
| `postgres.public_access` | boolean | default `false` | TF | In place. |
| `postgres.engine_version` | string | — | TF | In-place upgrade. Minor versions are left to AWS auto-patching and not reverted. |
| `postgres.min_capacity` | number (ACU) | Aurora Serverless v2 only | TF | `0` is valid and means "pause when idle". Presence must be tested, not truthiness. |
| `postgres.max_capacity` | number (ACU) | Aurora Serverless v2 only | TF | |
| `postgres.instance_class` | string | RDS only; default `db.t4g.micro` | TF | In-place modify with a reboot. |
| `postgres.allocated_storage` | number (GB, 20–65536) | RDS only | TF | Can grow in place. Cannot shrink. |
| `postgres.storage_type` | string | RDS only; `gp3` is the only option | TF | |
| `postgres.multi_az` | boolean | RDS only; default `false` | TF | In place, causes a failover. |
| `postgres.storage_encrypted` | boolean | RDS only; default `true` | TF-REPLACE | Encryption cannot be toggled on an existing instance. |
| `postgres.deletion_protection` | boolean | default `false` | TF | Must be `false` before a destroy succeeds. |
| `postgres.skip_final_snapshot` | boolean | — | TF | Read only at destroy time. |
| `postgres.iam_database_authentication_enabled` | boolean | default `false` | TF | In place. |
| `endpoint` | string | — | RO | Changes on replacement. |
| `port` | int | — | RO | |
| `isAurora` | boolean | — | RO | Reflects what is deployed, not what is configured. |
| `status` | string | — | RUNTIME | available / modifying / backing-up. |
| `engine` | string | — | RO | |
| `engineVersion` | string | — | RO | Deployed version, which may exceed the configured minor version. |
| Master password | string | — | RO | Generated and stored in the parameter store. |

---

## 11. `s3`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.bucket_postfix` | string | optional | TF-REPLACE | Bucket names are immutable. |
| `workload.bucket_public` | boolean | default `false` | TF | Applies to the backend bucket. |
| `buckets[].name` | string | required | TF-REPLACE | Immutable in AWS. |
| `buckets[].public` | boolean | default `false` | TF | |
| `buckets[].versioning` | boolean | default `false` | TF | Can be suspended but never fully removed once enabled. |
| `buckets[].cors_rules[].allowed_headers` | string[] | optional | TF | |
| `buckets[].cors_rules[].allowed_methods` | string[] | optional | TF | |
| `buckets[].cors_rules[].allowed_origins` | string[] | optional | TF | |
| `buckets[].cors_rules[].expose_headers` | string[] | optional | TF | |
| `buckets[].cors_rules[].max_age_seconds` | number | optional | TF | |
| Bucket ARN | string | — | RO | |
| Regional domain name | string | — | RO | |
| `creationDate` | string | — | RO | |
| `consoleUrl` | string | — | RO | Derived. |
| Object listing | object[] | — | RUNTIME | |
| Object **content** | string | — | LIVE | Written and deleted directly. |
| `AWS_S3_BUCKET` env var | string | — | RO | Injected into the backend task automatically. |

---

## 12. `eventbridge`

No stored configuration of its own. It is the routing fabric for event tasks.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| Bus name | string | — | RO | Default bus. |
| Rule list | object[] | derived from event tasks | TF | |
| Test event `source` | string | — | RUNTIME | Action input, not persisted. |
| Test event `detailType` | string | — | RUNTIME | Action input. |
| Test event `detail` | Record\<string,unknown\> | — | RUNTIME | Action input. |
| `eventId` | string | — | RO | Response value. |

---

## 13. `event-task`

One node per event-driven task.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `name` | string | required | CREATE / TF-REPLACE | Identity key. |
| `enabled` | boolean | default `true` | TF-REPLACE | |
| `rules[].name` | string | preferred multi-rule form | TF | |
| `rules[].sources` | string[] | preferred form | TF | Event pattern `source`. |
| `rules[].detail_types` | string[] | preferred form | TF | Event pattern `detail-type`. |
| `rule_name` | string | legacy; ignored when `rules[]` present | TF | |
| `sources` | string[] | legacy | TF | |
| `detail_types` | string[] | legacy | TF | |
| `docker_image` | string | optional | TF-DEPLOY | |
| `container_command` | string[] | optional | TF-DEPLOY | Array here. |
| `cpu` | number | Fargate-valid pairs | TF-DEPLOY | |
| `memory` | number | must pair with CPU | TF-DEPLOY | |
| `environment_variables` | Record\<string,string\> | optional | TF-DEPLOY | |
| `ecr_config.*` | see `service` | — | TF-REPLACE / TF-DEPLOY | |
| Parameter values | string | — | LIVE-RESTART | Add/remove is TF-DEPLOY. |
| Task definition ARN | string | — | RO | |
| Rule ARN | string | — | RO | |
| Last invocation | string | — | RUNTIME | |
| Failure count | int | — | RUNTIME | |

---

## 14. `scheduled-task`

One node per scheduled task.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `name` | string | required | CREATE / TF-REPLACE | Identity key; also names the schedule group. |
| `enabled` | boolean | default `true` | TF | Whether the task exists in AWS at all. |
| `auto_deploy` | boolean | absent means `true` | TF | Distinct from `enabled`. Outside dev no automatic trigger reaches a scheduled task, so `true` there enables only the manual path. |
| `schedule` | string | required | TF | `cron(...)` or `rate(...)`. In-place update. |
| `docker_image` | string | optional | TF-DEPLOY | |
| `container_command` | string | optional | TF-DEPLOY | String here, unlike services and event tasks. |
| `cpu` | number | Fargate-valid pairs | TF-DEPLOY | |
| `memory` | number | must pair with CPU | TF-DEPLOY | |
| `environment_variables` | Record\<string,string\> | optional | TF-DEPLOY | |
| `ecr_config.*` | see `service` | — | TF-REPLACE / TF-DEPLOY | |
| Parameter values | string | — | LIVE-RESTART | Add/remove is TF-DEPLOY. |
| Schedule group name | string | — | RO | |
| Scheduler role ARN | string | — | RO | |
| Last run time | string | — | RUNTIME | |
| Last run status | string | — | RUNTIME | |

---

## 15. `sns`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.setup_fcnsns` | boolean | default `false` | TF | Creates the platform application and topics. |
| `workload.slack_webhook` | string | optional | TF | Shared with the `ecs` node. |
| Topic ARN | string | — | RO | |
| Platform application ARN | string | — | RO | |
| FCM server key | string | — | RO | Supplied out of band via the parameter store. |

---

## 16. `sqs`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `sqs.enabled` | boolean | required when present | TF-REPLACE | Node appears only when `true`. |
| `sqs.name` | string | optional | TF-REPLACE | Queue names are immutable in AWS. |
| Queue URL | string | — | RO | |
| Queue ARN | string | — | RO | |
| Approximate message count | int | — | RUNTIME | |
| Consumer count | int | derived from scheduled and event tasks | RO | |

---

## 17. `ses`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `ses.enabled` | boolean | required | TF-REPLACE | |
| `ses.domains[].domain` | string | preferred multi-domain form | TF-REPLACE | Array key; verification identity. |
| `ses.domains[].zone_id` | string | optional | TF | Enables automatic DKIM/SPF/DMARC record creation. |
| `ses.domains[].enable_mail_from` | boolean | per-domain override | TF | Falls back to the global setting. |
| `ses.domains[].mail_from_subdomain` | string | only when MAIL FROM enabled | TF | |
| `ses.domains[].dmarc_policy` | `"none"` \| `"quarantine"` \| `"reject"` | per-domain override | TF | |
| `ses.domains[].dmarc_rua_email` | string | per-domain override | TF | |
| `ses.global_enable_mail_from` | boolean | default for all domains | TF | |
| `ses.global_mail_from_subdomain` | string | default for all domains | TF | |
| `ses.global_dmarc_policy` | `"none"` \| `"quarantine"` \| `"reject"` | default for all domains | TF | |
| `ses.global_dmarc_rua_email` | string | default for all domains | TF | |
| `ses.test_emails` | string[] | account-wide | TF | Verified identities for sandbox mode. |
| `ses.domain_name` | string | legacy single-domain | TF-REPLACE | Superseded by `domains[]`. |
| `inSandbox` | boolean | — | RUNTIME | Account-level, not per-environment. |
| `sendingEnabled` | boolean | — | RUNTIME | |
| `dailyQuota` | int | — | RUNTIME | AWS-controlled. |
| `maxSendRate` | number | — | RUNTIME | |
| `sentLast24Hours` | int | — | RUNTIME | |
| `verifiedDomains` | string[] | — | RUNTIME | Actual state, which lags DNS propagation. |
| `verifiedEmails` | string[] | — | RUNTIME | |
| `suppressionListEnabled` | boolean | — | RUNTIME | |
| `reputationStatus` | string | — | RUNTIME | |
| DKIM CNAME tokens | string[] | — | RO | Assigned at identity creation. |
| Production-access request | — | — | LIVE (action) | A support case, not infrastructure. |
| Test email send | — | — | LIVE (action) | |

---

## 18. `cloudwatch`

Read-only.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| Log group names | string[] | — | RO | |
| Log retention | int (days) | fixed `30` | TF | Module constant. |
| Log events | object[] | — | RUNTIME | |
| Metric series | object | — | RUNTIME | CPU, memory, invocations. |
| Dashboard URL | string | — | RO | Derived. |

---

## 19. `xray`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `workload.xray_enabled` | boolean | default `false` | TF-DEPLOY | Backend sidecar. |
| `services[].xray_enabled` | boolean | per service, default `false` | TF-DEPLOY | |
| Sampling rules | object[] | — | TF | Module-defined. |
| Trace list | object[] | — | RUNTIME | |
| Service map | object | — | RUNTIME | |
| Daemon sidecar status | string | — | RUNTIME | |

---

## 20. `secrets-manager`

Despite the node name, this is Systems Manager Parameter Store. It is the only node whose primary
properties are live rather than redeploy-gated.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `name` | string | required | LIVE, create-only | Full path. AWS cannot rename a parameter — a rename is delete plus create, and both halves need a redeploy so the task definition picks up the change. |
| `value` | string | required | LIVE-RESTART | No redeploy. Running tasks keep the old value until they restart. |
| `type` | `"String"` \| `"StringList"` \| `"SecureString"` | required | LIVE, create-only | AWS rejects a type change on an existing parameter. |
| `description` | string | optional | LIVE | Metadata only; no task impact. |
| `overwrite` | boolean | request flag | — | Not stored. |
| `version` | int | — | RO | Incremented per write. |
| `arn` | string | — | RO | |
| `lastModifiedDate` | string (ISO) | — | RO | |
| Parameter scope | derived | from services, scheduled tasks, event tasks, pgAdmin, SNS, Postgres | RO | The panel builds the path list from what exists; it does not create scopes. |
| IAM read grant | policy | — | TF | Wildcard on the prefix, so a new parameter under an existing prefix needs no IAM change — only the task-definition redeploy. |

---

## 21. `efs`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `efs[].name` | string | required | TF-REPLACE | Array key. |
| `efs[].path` | string | required | TF | Access-point root directory. |
| `workload.efs[].name` | string | must match a defined volume | TF-DEPLOY | Which file system the backend mounts. |
| `workload.efs[].mount_point` | string | required per mount | TF-DEPLOY | Container path. |
| File system ID | string | — | RO | |
| Access point ID | string | — | RO | |
| Mount target IPs | string[] | — | RO | One per availability zone. |
| Size in bytes | int | — | RUNTIME | |

---

## 22. `appsync`

Read-only on the canvas.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `pubsub_appsync.enabled` | boolean | required | TF-REPLACE | |
| `pubsub_appsync.schema` | boolean | optional | TF | Deploy the bundled schema. |
| `pubsub_appsync.resolvers` | boolean | optional | TF | Deploy the bundled resolvers. |
| `pubsub_appsync.auth_mode` | `"cognito"` \| `"oidc"` \| `"lambda"` | absent means `"lambda"` | TF | Changes the API's primary authorization type. |
| `pubsub_appsync.api_key_enabled` | boolean | default `false` | TF | An API key bypasses `auth_mode` entirely — whoever holds it reaches every resolver without a token. Existing environments were migrated to `true` to preserve a key they already had. |
| `pubsub_appsync.auth_lambda` | boolean | only when mode is `lambda` | TF-DEPLOY | Package a project-supplied authorizer instead of the bundled one. |
| `pubsub_appsync.cognito_app_id_client_regex` | string | only when mode is `cognito` | TF | Pipe-separated client IDs. Unset accepts every app client in the pool, and the pool carries web, mobile and dashboard clients. |
| `pubsub_appsync.oidc_issuer` | string | required when mode is `oidc` | TF | On an OIDC-only API, AppSync does not compare the token's `iss` against this — only the signature against the issuer's keys. |
| `pubsub_appsync.oidc_client_id` | string | only when mode is `oidc` | TF | Matched against `aud`, falling back to `azp`. Pipe-separated. |
| `pubsub_appsync.jwks_uri` | string (https) | required when mode is `lambda` | TF-DEPLOY | No default — an unset value used to mean trusting a hardcoded third party. |
| `pubsub_appsync.jwt_issuer` | string | only when mode is `lambda` | TF-DEPLOY | Expected `iss`, comma-separated. |
| `pubsub_appsync.jwt_audience` | string | only when mode is `lambda` | TF-DEPLOY | Expected `aud`, comma-separated. |
| `pubsub_appsync.required_claims` | Record\<string,string[]\> | only when mode is `lambda`; rejected in any other mode | TF-DEPLOY | Claim name to accepted values; empty list means "must be present". For policy claims, not identity. |
| GraphQL endpoint URL | string | — | RO | |
| Realtime endpoint | string | — | RO | |
| API ID | string | — | RO | |
| API key value | string | only when the key is enabled | RO | AWS-generated, expires. |

---

## 23. `amplify`

One node per app.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `name` | string | required | TF-REPLACE | Array key. |
| `github_repository` | string | required | TF | |
| `subdomain_prefix` | string | optional | TF | |
| `custom_domain` | string | optional | TF | Requires DNS validation. |
| `environment_variables` | Record\<string,string\> | app-level | TF | Applied to all branches; picked up on the next build. |
| `spa_mode` | boolean | default `false` | TF | 200 rewrite instead of 404 to 200. |
| `branches[].name` | string | required | TF-REPLACE | Branch key. |
| `branches[].stage` | `"PRODUCTION"` \| `"DEVELOPMENT"` \| `"BETA"` \| `"EXPERIMENTAL"` | optional | TF | |
| `branches[].enable_auto_build` | boolean | optional | TF | Build on push. |
| `branches[].enable_pull_request_preview` | boolean | optional | TF | |
| `branches[].environment_variables` | Record\<string,string\> | optional | TF | Overrides app-level. |
| `branches[].custom_subdomains` | string[] | requires a custom domain | TF | |
| App ID | string | — | RO | |
| Default domain | string | — | RO | |
| Build job ID | string | — | RUNTIME | |
| Build status | string | — | RUNTIME | |
| Build logs | string | — | RUNTIME | |
| Domain verification status | string | — | RUNTIME | |
| Trigger build | — | — | LIVE (action) | No redeploy. |

---

## 24. `cloudfront`

One node per distribution.

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| `name` | string | required | TF-REPLACE | Array key. |
| `enabled` | boolean | required | TF | `false` hides the node and disables the distribution. |
| `price_class` | `"PriceClass_100"` \| `"PriceClass_200"` \| `"PriceClass_All"` | default `PriceClass_100` | TF | In place; propagation takes roughly 15 minutes. |
| `default_root_object` | string | default `index.html` | TF | |
| `spa_mode` | boolean | default `false` | TF | 404 to `index.html`. |
| `domain_aliases` | string[] | requires a matching certificate in `us-east-1` | TF | |
| `origins[].name` | string | required | TF | Referenced by cache behaviors. |
| `origins[].type` | `"s3"` \| `"amplify"` \| `"alb"` \| `"custom"` | required | TF | Drives which fields below apply. |
| `origins[].domain_name` | string | required for `custom` and `alb`; auto-resolved for `amplify` and `s3` | TF | |
| `origins[].origin_path` | string | optional | TF | |
| `origins[].protocol_policy` | `"https-only"` \| `"http-only"` \| `"match-viewer"` | non-S3 origins | TF | |
| `origins[].custom_headers` | Record\<string,string\> | optional | TF | |
| `origins[].bucket_name` | string | only when type is `s3` | TF | |
| `origins[].create_bucket` | boolean | only when type is `s3` | TF-REPLACE | Creates a new bucket for this origin. |
| `origins[].use_oac` | boolean | only when type is `s3` | TF | Origin Access Control. |
| `origins[].amplify_app_name` | string | only when type is `amplify` | TF | Must match an Amplify app name. |
| `cache_behaviors[].path_pattern` | string | required | TF | |
| `cache_behaviors[].origin_name` | string | must match an origin name | TF | |
| `cache_behaviors[].allowed_methods` | string[] | optional | TF | |
| `cache_behaviors[].cached_methods` | string[] | optional | TF | |
| `cache_behaviors[].forward_query_string` | boolean | optional | TF | |
| `cache_behaviors[].forward_headers` | string[] | optional | TF | |
| `cache_behaviors[].forward_cookies` | `"none"` \| `"whitelist"` \| `"all"` | optional | TF | |
| `cache_behaviors[].viewer_protocol_policy` | `"redirect-to-https"` \| `"https-only"` \| `"allow-all"` | optional | TF | |
| `cache_behaviors[].min_ttl` | number (s) | optional | TF | |
| `cache_behaviors[].default_ttl` | number (s) | optional | TF | |
| `cache_behaviors[].max_ttl` | number (s) | optional | TF | |
| `cache_behaviors[].compress` | boolean | optional | TF | |
| `logging.enabled` | boolean | required when logging present | TF | |
| `logging.bucket_name` | string | required when logging enabled | TF | |
| `logging.prefix` | string | optional | TF | |
| `logging.include_cookies` | boolean | optional | TF | |
| `additional_zones[].domain` | string | deprecated | TF | Use the centralized additional domains instead. |
| `additional_zones[].zone_id` | string | deprecated | TF | |
| `additional_zones[].create_zone` | boolean | deprecated | TF | |
| Distribution ID | string | — | RO | |
| Distribution domain name | string | — | RO | |
| Distribution ARN | string | — | RO | |
| Deployment status | string | — | RUNTIME | InProgress / Deployed. |

---

## 25. `custom-terraform`

| Property | Type | Conditional | Class | Notes |
|---|---|---|---|---|
| File name | string | required | TF | |
| File content (HCL) | string | required | TF | Free-form. Not validated by meroku — a syntax error surfaces at plan time. |
| Module list | object[] | — | RUNTIME | Parsed from the files present. |
| Bridge variables | Record\<string,string\> | — | RO | Outputs exposed to custom modules: VPC ID, subnet IDs, cluster ARN and so on. |
| Editor buffer | string | — | UI | |
| Cursor position | object | — | UI | |

---

## 26. `alarms`

Currently inert — the node renders but the panel is disabled.

| Property | Type | Conditional | Class |
|---|---|---|---|
| Alarm name | string | — | RO |
| Metric | string | — | TF |
| Threshold | number | — | TF |
| Comparison operator | string | — | TF |
| Alarm state | `"OK"` \| `"ALARM"` \| `"INSUFFICIENT_DATA"` | — | RUNTIME |
| SNS action target | string (ARN) | requires SNS or a Slack webhook configured | TF |

---

## Layout nodes: `group` and `dynamicGroup`

| Property | Type | Class |
|---|---|---|
| `label` | string | UI |
| `position.x` | number | UI |
| `position.y` | number | UI |
| `style.width` | number | UI |
| `style.height` | number | UI |
| `edgeHandles[].sourceHandle` | string | UI |
| `edgeHandles[].targetHandle` | string | UI |

Stored per environment on the meroku host. Never reaches AWS.

---

## Cross-cutting notes

**Desired count drift.** Task definitions are ignored on redeploy, so CI/CD image pushes survive.
`desired_count` is not ignored. If autoscaling moved a service to 5 and the model says 1, the next
redeploy scales it back to 1.

**Type inconsistency across node families.** `workload.backend_cpu` and `backend_memory` are strings;
`services[].cpu`, `scheduled_tasks[].cpu` and `event_processor_tasks[].cpu` are numbers.
`scheduled_tasks[].container_command` is a string while every other `container_command` is a string
array.

**Array keys are identity.** Every `name` inside an array — services, scheduled tasks, event tasks,
Amplify apps, CloudFront distributions, buckets, EFS volumes, SES domains — is the resource identity
key. Renaming is always destroy-and-create, never a rename.

**`false` and `0` are meaningful.** Zero and false are valid values, not "missing". Notably
`postgres.min_capacity`, where `0` means "pause when idle" and must be distinguished from an absent
value.

**Saved does not mean deployed.** Any property in a TF class shows intent, not deployed state. The
RUNTIME and RO columns are the only source of deployed truth.

**Secrets.** `workload.slack_webhook`, `workload.backend_env_variables` and
`services[].environment_variables` are all stored and rendered in plaintext. Anything sensitive
belongs on the `secrets-manager` node, where the value is never stored alongside the model.

---

## Which class does a change fall into?

```
Is it a stored model property?
├─ No
│  ├─ Parameter value                            → LIVE-RESTART
│  ├─ S3 object content                          → LIVE-RESTART
│  ├─ Canvas layout                              → UI
│  ├─ AWS-assigned identifier                    → RO
│  └─ Counts, status, utilization                → RUNTIME
└─ Yes → a Terraform redeploy is required. Which flavour?
   ├─ project / env / region / state settings    → CREATE       (a different stack, not a change)
   ├─ An array entry's `name`                    → TF-REPLACE
   ├─ Bucket name or postfix, queue name,
   │  storage_encrypted, aurora toggle, dbname,
   │  username, vpc_cidr, use_default_vpc        → TF-REPLACE   (data loss risk)
   ├─ Anything inside a task definition:
   │  image, command, cpu, memory, ports,
   │  env vars, parameter list, sidecars         → TF-DEPLOY    (rolling restart)
   └─ Everything else                            → TF           (in place)
```
