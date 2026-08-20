# ECS Egress Cost Model

Why meroku puts a public IPv4 address on every ECS task, what that costs as a
project grows, and when to switch to a NAT Gateway instead.

This document is the reasoning behind `app/egress_advisor.go`. If you change the
thresholds there, change them here too.

## The question

An ECS task in `awsvpc` mode needs outbound network access to be useful at all:
it has to pull its image from ECR, ship logs to CloudWatch, read secrets, and
call whatever third-party APIs the application uses. AWS gives five ways to
provide that, and they are priced on completely different axes.

meroku currently hardcodes one of them (`assign_public_ip = true`, in
`modules/workloads/backend.tf`, `services.tf`, `pgadmin.tf`,
`modules/ecs_task/main.tf` and `modules/event_bridge_task/main.tf`). That is the
right default at small scale and the wrong one past a threshold. This document
establishes where the threshold is.

## Unit rates

us-east-1 list prices, August 2026. A month is 730 hours.

| Billable object | Rate | Monthly |
|---|---|---|
| Public IPv4 address | $0.005 / hr | $3.65 |
| NAT Gateway | $0.045 / hr | $32.85 |
| NAT Gateway data processing | $0.045 / GB | — |
| Interface VPC endpoint (per AZ) | $0.01 / hr | $7.30 |
| Interface endpoint data processing | $0.01 / GB | — |
| Cross-AZ data transfer | $0.01 / GB each way | — |
| Internet data transfer out | $0.09 / GB | — |
| S3 / DynamoDB gateway endpoint | free | $0 |
| Internet Gateway | free | $0 |
| Egress-only Internet Gateway | free | $0 |
| IPv6 address | free | $0 |
| HTTP API (v2) VPC Link | free | $0 |

Two of these are worth stating explicitly because they are commonly assumed to
cost money and do not:

- **HTTP API VPC Links are free.** REST API (v1) VPC links need a Network Load
  Balancer and cost real money; the v2 links meroku creates in
  `modules/workloads/api_gateway.tf` carry no hourly charge. This was confirmed
  against a real AWS bill: no VPC-link usage type appears on it.
- **Gateway endpoints for S3 and DynamoDB are free**, unlike interface
  endpoints. There is never a cost reason not to create the S3 one.

Regional variation is real and material. ap-southeast-2 charges $0.059/hr for a
NAT Gateway against $0.045 in us-east-1, so a Sydney NAT costs $43/mo rather
than $32.85. The advisor uses us-east-1 rates; treat its output as a floor.

Internet data-transfer-out is excluded from every comparison below, because
every option pays it identically and it cancels.

## Which dimension each option scales on

This is the whole model. Each option is expensive in exactly one dimension and
free in the others.

| Option | Fixed driver | Tasks | AZs | Regions | Traffic |
|---|---|---|---|---|---|
| Public IPv4 | $3.65 / task | linear | flat | linear | flat |
| NAT Gateway | $32.85 / NAT | flat | linear\* | linear | linear |
| NAT instance | ~$7.36 / instance | flat | linear\* | linear | stepped |
| Interface endpoints | $7.30 / endpoint / AZ | flat | linear | linear | mild |
| IPv6 + egress-only IGW | none | flat | flat | flat | flat |

\* Only when run one-per-AZ for redundancy.

The two consequences that matter:

- **Public IPv4 has no per-GB component at all.** Its cost is identical at 50 GB
  and at 50 TB. It buys capacity per task and charges nothing for throughput.
- **NAT Gateway has no per-task component at all.** One NAT serves two tasks or
  two hundred for the same hourly rate, then charges for every byte.

So growing services favours NAT, and growing traffic favours public IPs. They
cross, and the crossing point moves.

## Growth curves

Two tasks per service, 2 AZs, single NAT, traffic ramping with size.

| Services | Tasks | Traffic | Public IPv4 | NAT (single) | NAT (HA) | IPv6 |
|---|---|---|---|---|---|---|
| 1 | 2 | 20 GB | **$7.30** | $33.95 | $66.60 | $0 |
| 2 | 4 | 30 GB | **$14.60** | $34.50 | $67.05 | $0 |
| 3 | 6 | 50 GB | **$21.90** | $35.60 | $67.95 | $0 |
| 5 | 10 | 80 GB | **$36.50** | $37.25 | $69.30 | $0 |
| 8 | 16 | 120 GB | $58.40 | **$39.45** | $71.10 | $0 |
| 12 | 24 | 180 GB | $87.60 | **$42.75** | $73.80 | $0 |
| 20 | 40 | 300 GB | $146.00 | **$49.35** | $79.20 | $0 |

At 20 services, moving from public IPs to a single NAT saves **$96.65/mo
($1,160/yr)** per environment.

Holding services fixed at 3 and growing traffic instead shows the opposite
shape — public IPv4 stays at $21.90 while a NAT Gateway climbs to $2,782.85 at
50 TB/mo.

## Where the lines cross

Tasks at which a NAT Gateway becomes cheaper than public IPv4, at 2 AZs:

| Traffic | Single NAT wins above | HA NAT wins above |
|---|---|---|
| 20 GB | 9.3 tasks (5 services) | 18.2 tasks (10 services) |
| 50 GB | 9.8 tasks (5 services) | 18.6 tasks (10 services) |
| 100 GB | 10.5 tasks (6 services) | 19.2 tasks (10 services) |
| 200 GB | 12.0 tasks (7 services) | 20.5 tasks (11 services) |
| 500 GB | 16.5 tasks (9 services) | 24.2 tasks (13 services) |

More traffic pushes the threshold **up**, because the NAT charges per GB and a
public IP does not.

The crossing is gentle, not a cliff. At 5 services the two options are $36.50
against $37.25 — under a dollar apart. Being a service or two late costs
pennies, so the advisory does not need to be urgent or blocking.

## The decision

**Two phases, with the switch driven by service count.**

| Phase | When | Strategy | Cost |
|---|---|---|---|
| 1 | 1–4 services | public IPv4 per task | $7–29 /mo |
| 2 (non-prod) | 5+ services | one NAT Gateway | ~$35–49 /mo flat |
| 2 (prod) | 10+ services | one NAT Gateway per AZ | ~$68–79 /mo flat |

Non-prod switches earlier because a single NAT is acceptable there; a NAT
Gateway is zonal, so one NAT means one AZ's failure takes out egress. Production
carries the HA cost, which pushes its break-even to roughly 10 services.

### Why not the other options

- **NAT instance** (fck-nat and similar) is the cheapest option at every size —
  about $7.36/mo flat, with no per-GB charge. It is rejected on operational
  grounds, not price: it is an EC2 instance to patch, monitor, and fail over,
  which conflicts with meroku's goal of generating infrastructure that needs no
  babysitting. Reconsider if someone needs private subnets without IPv6.
- **Interface endpoints** cost $58.40/mo fixed at 4 endpoints across 2 AZs
  before a byte moves, and still provide no internet access. They are a
  compliance tool for no-internet-egress requirements, never a savings tool at
  this scale.
- **IPv6 + egress-only IGW** is free in every dimension and is the only option
  that needs no switch ever. See below.

### Secondary effect: the switch improves security

In phase 1 each task holds a routable public address. Inbound is blocked by
security group, but the address exists and is reachable by scanners. In phase 2
the tasks sit in private subnets with no public address at all. The cheaper
option at scale is also the safer one, so the two criteria do not trade off.

## Prerequisites for a cheap switch

The switch is only worth recommending if it is cheap to perform. Three things
have to be true before phase 1 ships, or the recommendation asks for a
re-architecture of a running environment.

1. **The VPC creates private subnets from day one.** `modules/vpc/main.tf`
   currently creates public subnets only. Subnets and route tables cost nothing
   while empty — only the NAT Gateway costs money — so creating them in phase 1
   is free and makes phase 2 a config change.
2. **`assign_public_ip` and subnet placement become one setting.** It is
   currently hardcoded `true` in five files, so today the switch is a code
   change rather than a YAML change.
3. **The free S3 gateway endpoint ships from day one.** It does nothing in
   phase 1. In phase 2 it keeps ECR image-layer pulls off the NAT entirely,
   which matters because image pulls are the largest component of task egress
   and routing them through a NAT costs $0.045/GB for no benefit.

None of these are implemented yet. The advisor recommends the switch; it does
not perform it.

## The IPv6 endgame

IPv6 with an egress-only Internet Gateway costs **$0 at every service count,
every traffic level, every AZ count and every region**. It needs no switch, it
is fully AWS-native, and it is outbound-only by construction, so it is also the
most secure option. On cost, ops burden, and security it dominates both phases.

AWS-side support is complete: ECS IPv6-only went GA in September 2025, and ECR,
CloudWatch Logs, Secrets Manager and S3 all publish dualstack endpoints. Tasks
must use dualstack ECR image URIs.

The single blocker is third-party reachability. Any external API without an AAAA
record is unreachable from an IPv6-only task, and the workaround — NAT64 — runs
on a NAT Gateway and costs phase-2 money anyway.

That makes IPv6 a per-project determination rather than a default: list the
external hostnames a project calls and check each for an AAAA record. If they
all have one, that project skips the two-phase plan entirely.

## Sources

- [Amazon VPC pricing](https://aws.amazon.com/vpc/pricing/)
- [Amazon ECS announces IPv6-only support](https://aws.amazon.com/blogs/containers/amazon-ecs-announces-ipv6-only-support/)
- [CloudWatch Logs IPv6 support](https://aws.amazon.com/about-aws/whats-new/2024/02/amazon-cloudwatch-logs-ipv6/)
- [Secrets Manager IPv6 support](https://aws.amazon.com/about-aws/whats-new/2024/12/ipv6-compatibility-aws-secrets-manager-vpc-endpoints)
- [Egress-only internet gateways](https://docs.aws.amazon.com/vpc/latest/userguide/egress-only-internet-gateway.html)
