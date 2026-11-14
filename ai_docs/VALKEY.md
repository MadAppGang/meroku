# Valkey

An environment can have one serverless Valkey cache (ElastiCache Serverless,
engine `valkey`), created by `modules/valkey`. The workloads listed in
`valkey.access` can reach it; nothing else can. No custom Terraform is needed.

## YAML

```yaml
valkey:
  enabled: true
  engine_version: "8"   # "7" | "8" | "9", default "8"
  max_storage_gb: 1     # ceiling on stored data, whole GB, at least 1, default 1
  access: [api, nightly]
```

- `access`: workload names — `backend`, or the name of a service, scheduled
  task or event task. Each at most once.
- `meroku generate` refuses an engine version other than 7, 8 or 9, a storage
  cap below 1 or not a whole number, a name listed twice, and a name that is not
  a workload. A switched-off workload (`enabled: false`) is dropped with a
  warning. An enabled cache with no workload in `access` is a warning: it bills
  and nothing can use it.
- Absent, or `enabled: false`, renders nothing. There is no schema migration.

The web UI has a Valkey node on the canvas (on/off, version, storage cap and an
access picker). Deleting a service or task in the UI removes it from `access`.

## What a listed workload gets

| Variable | Value |
|---|---|
| `VALKEY_HOST` | the cache endpoint's address |
| `VALKEY_PORT` | the endpoint's port (6379) |
| `VALKEY_URL` | `rediss://<address>:<port>` |

They reach the container through `extra_env` (`local.workload_extra_env` in
`env/main.hbs`), after the workload's own variables. A workload in `access`
whose YAML env vars set any of the three names is refused at generate
(`app/workload_env_reserved.go`). There are no secrets: the cache has no
password.

## Access: security groups only

The cache has its own security group with no inline rules and no egress. For
every distinct security group the listed workloads are seen as, it gets one
standalone ingress rule on 6379 (primary endpoint) and one on 6380 (reader
endpoint). The workloads' own security groups are not changed.

Rules are keyed by the workload contract's `sg_key`, not by workload name:

- A Fargate (awsvpc) workload is `workload:<name>`, with its own security group.
- A workload on a **bridge-mode EC2 pool** — the backend through
  `local.backend_bridge`, a service through `local.service_bridge[<name>]` — has
  no network interface of its own; its traffic leaves from the EC2 instance. It
  is `ecs_instance`, seen as the instance security group
  (`aws_security_group.ecs_instance`). All bridge-mode workloads share that one
  key, so they get one rule pair between them (a second identical rule would be
  refused as `InvalidPermission.Duplicate`).

**Bridge-mode caveat.** Because the rule allows the instance security group,
every task on a bridge-mode instance can reach Valkey, listed or not. Only the
listed ones get the variables, but an unlisted task on the same instance that
knows the endpoint can connect. A workload that must be isolated from Valkey
runs on Fargate or on an `awsvpc` pool.

## The cache

| Setting | Value |
|---|---|
| Name | `<project>-valkey-<env>` through `modules/naming` (limit 40), lower-cased with `_` turned into `-`, because CreateServerlessCache accepts only lower-case letters, digits and hyphens |
| Engine | `valkey`, `major_engine_version` from `engine_version` |
| Network | `connection_type = "vpc"`, the environment's `local.subnet_ids` |
| Usage limits | `data_storage.maximum = max_storage_gb` GB; `minimum` is never set; no ECPU limit |
| Encryption | in transit (TLS, always on for serverless) and at rest with the AWS-owned key |

`local.subnet_ids` is the public subnet set of a custom VPC (two AZs). The
endpoint has no public address, whatever subnet it is in; only the VPC can reach
it. In an environment on the default VPC it is every default subnet.

## Cost

Serverless Valkey bills by the hour for stored data and for ECPUs (compute per
request), with a minimum of 100 MB of storage. At us-east-1 prices:

- Storage: about **$61 per GB-month** ($0.084 per GB-hour).
- Floor: the 100 MB minimum is about **$6 a month** for an idle cache.
- ECPUs: about $0.0023 per million; a small app adds cents.

`max_storage_gb` is a ceiling, so it caps the storage bill. At the ceiling the
cache evicts keys that have a TTL, least recently used first, and when nothing
can be evicted writes fail with an out-of-memory error; the bill does not grow. **`minimum` is never
set**, because a minimum bills that much storage every hour whether used or not:
a 1 GB minimum raises the floor from about $6 to about $61 a month.

## Clients

Serverless Valkey is a cluster endpoint behind a proxy and always requires TLS.

- Use a **cluster-aware client with TLS** (`rediss://`): e.g. ioredis
  `Cluster` with `redisOptions.tls`, node-redis `createCluster`, redis-py
  `RedisCluster(ssl=True)`, go-redis `ClusterClient` with `TLSConfig`. A plain
  non-TLS client cannot connect.
- **Multi-key commands must hash to one slot.** A command whose keys land in
  different slots fails with `CROSSSLOT`. Put related keys in one slot with a
  hash tag: `{user:42}:profile` and `{user:42}:cart` share the slot of `user:42`.
- **No `PSUBSCRIBE` and no keyspace notifications.** Pattern subscriptions are
  refused, and `notify-keyspace-events` cannot be set, so expiry or change
  events are not available.
- **Pub/sub: use sharded pub/sub** (`SSUBSCRIBE`/`SPUBLISH`). It is
  at-most-once: a subscriber that is disconnected, restarting or slow misses the
  message, and nothing is stored. Use the event bus (EventBridge and a queue) for
  anything that must arrive.

## Custom Terraform

`local.bridge` carries `valkey_address`, `valkey_port` and
`valkey_security_group_id`, all `""` when Valkey is off. A client meroku does not
manage (a Lambda in the VPC, a bastion) is let in from custom Terraform with an
`aws_vpc_security_group_ingress_rule` on `local.bridge.valkey_security_group_id`.
ACL users and passwords are not offered; a project that needs them builds them
in custom Terraform.

## Code

- `app/valkey.go` — `validateValkeyConfigMap`, writes `_valkey` for the template.
- `app/model.go` — `Env.Valkey`.
- `env/main.hbs` — `module "valkey"` and its line in `local.workload_extra_env`.
- `modules/valkey/` — the cache, its security group and the ingress rules.
- `modules/workloads/workload_contract.tf` — `security_group_id` and `sg_key`.
- `web/src/components/ValkeyNodeProperties.tsx` — the canvas node's panel.
