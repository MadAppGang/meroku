# VPC CIDR Allocation

How meroku chooses, validates and checks each environment's VPC range (`vpc_cidr`).

## Why it exists

Before this change, every custom-VPC environment meroku created got `10.0.0.0/16`, and
`modules/vpc` had the same default. Two VPCs with overlapping ranges can never be peered or
attached to the same Transit Gateway: AWS refuses the peering and does not propagate the
route. A VPC's primary range cannot be changed either. Changing `vpc_cidr` on a deployed
environment replaces the VPC and everything in it: subnets, security groups, the ALB and its
target groups, the Aurora subnet group, and every ECS service.

So the range has to be right when the environment is created. meroku does three things:

1. **New environments get a random free /16**, written explicitly into the YAML and never
   re-randomised on load.
2. **`meroku generate` checks the rules** and refuses a range that breaks them. Overlap with
   another environment is reported as a warning.
3. **An explicit check against AWS** (`--aws`, or "Check my AWS accounts too" in the web UI)
   also avoids every VPC that already exists in the environment's account and region, and in the
   account and region of every other environment in the project.

Existing environments' `vpc_cidr` values are never rewritten. No migration touches them.

## The rules

Implemented once, in `validateVPCCIDR` (`app/vpc_cidr.go`). The CLI, `meroku generate`, the
web API and the web UI's manual-entry field all call it. The UI does not reimplement the rules.

Checked in this order. The first failure wins:

| Code | Rule | Example that fails |
|---|---|---|
| `invalid_format` | An IPv4 CIDR block | `10.42.0.0`, `abc` |
| `reserved_range` | Does not overlap 172.16.0.0/12 (Docker bridge 172.17, Docker pools, AWS default VPC 172.31, Cloud9/SageMaker), 169.254.0.0/16 (ECS task metadata and credentials) or 198.19.0.0/16 (restricted by AWS) | `172.20.0.0/16`, `172.20.1.0/16` |
| `not_private` | Inside 10.0.0.0/8 or 192.168.0.0/16 (RFC 1918) | `11.0.0.0/16` |
| `size` | Prefix length /16 to /19 | `10.42.0.0/20`, `10.0.0.0/8` |
| `not_network_address` | The host bits are zero. The error carries the corrected value as `fix` | `10.42.1.0/16` (fix: `10.42.0.0/16`) |

The range rules are checked on the value with its host bits cleared, and the network-address
rule comes last. So `fix` is offered only when the corrected value passes every other rule, and
"Use this" in the web UI never offers a range that fails again.

The size range has a reason at each end. /16 is the largest VPC AWS allows. The module carves
subnets with `cidrsubnet(vpc_cidr, 8, n)`, and /19 is the smallest VPC whose subnets are still
at least /27, which is the ALB minimum.

Each error has two texts. `message` is technical and goes to the CLI and logs. `hint` is plain
language for the web UI and never uses the words "CIDR", "mask" or "prefix". Range values appear
in a hint only as values, for example "Use 10.5.0.0/16 instead".

**Overlap is a warning, never an error.** Two environments that will never be connected can
share a range on purpose.

### Where each check runs

| Check | Where | When |
|---|---|---|
| Rules (errors) | `validateVPCCIDRMap` in `applyTemplate` (`app/deploy.go`) | Every `meroku generate` |
| Overlap with sibling envs (warning) | `warnOnVPCCIDROverlap` | Every `meroku generate`, offline |
| Overlap with VPCs in AWS (warning) | `inspectVPCCIDR` with AWS on | Only on request: `--aws`, the TUI prompt, `aws=1` |
| Terraform shape check | `modules/vpc/variables.tf` `validation` blocks | Every `terraform plan` |

`meroku generate` never calls AWS for this. When it prints a sibling overlap warning, it adds a
tip to run `meroku vpc-cidr check <env> --aws`.

Every rule error from generate ends with the same guidance: if the environment is already
deployed, do not change `vpc_cidr`. Add `allow_vpc_cidr_override: true` instead. If it is not
deployed yet, run `meroku vpc-cidr assign <env>`.

### Terraform validation

`modules/vpc/variables.tf` checks only the shape: IPv4 format, network address, and size /16
to /20. It deliberately does not check private or reserved ranges, because a `plan` must never
lock out a VPC that already exists. The Terraform bound is /20 rather than /19 because /20 is
the smallest range the module's subnet maths can build, so no deployable VPC is smaller than
that.

- The validation blocks need **Terraform ≥ 1.2.6**. Each function call is wrapped in `can()` or
  `try()`.
- The module's `.tftest.hcl` tests use `mock_provider` and need **Terraform ≥ 1.7**. CI runs 1.10.5.
- The module default stays `10.0.0.0/16`. Changing it would replace the VPC of every environment
  whose YAML never set the key.

## The pool

New ranges are drawn from every `10.N.0.0/16` except:

- `10.0.0.0/15` (10.0 and 10.1). This is the default in Copilot, SST, Pulumi awsx,
  terraform-aws-modules/vpc and Heroku. AWS also refuses a secondary range from 10.0.0.0/16 on
  a VPC whose primary range is in 10.0.0.0/15.
- `10.99.0.0/16`, Porter's documented default.

That leaves **253** ranges. A pick is uniform over the ranges that do not overlap any `used`
range:

- **Offline:** the effective range of every other environment YAML in the project directory.
- **With AWS:** also every IPv4 range of every VPC in the environment's account and region,
  including secondary ranges in every state except `disassociating`, `disassociated`, `failing`
  and `failed` (a missing state counts: a range we leave out is a range we might suggest). The
  primary range always counts. The environment's own VPC is not counted.
- **With AWS, sibling accounts:** also every VPC in the account and region of each other
  environment in the project, read with that environment's own `aws_profile` and `region`. This
  is the case the feature exists for: two environments in different accounts that will later be
  peered or joined by a Transit Gateway. Details below.

`pickVPCCIDRs` draws several distinct ranges without replacement, for the web UI's five-option
dropdown. When no range is free, it returns `errVPCCIDRPoolExhausted`.

### How random is safe enough

Random beats a fixed default, but it is not a guarantee across projects that cannot see each
other. These are the birthday-problem odds that at least two of *k* independently created VPCs
collide, with 253 ranges:

| VPCs | Chance of at least one collision |
|---|---|
| 2 | 0.4% |
| 5 | 3.9% |
| 10 | 16.5% |
| 20 | 53.7% |
| 30 | 83.3% |

Inside one project directory there is no chance involved: siblings are always avoided. The odds
apply across separate projects, which is what `--aws` covers when they share an account with one
of this project's environments.

### Sibling accounts

`listSiblingAccountVPCs` (`app/vpc_cidr_service.go`) runs after the environment's own listing:

- Siblings come from `siblingVPCCIDRAccounts`, the same raw-YAML scan as the offline siblings,
  including default-VPC siblings (their account can still hold networks to avoid).
- A sibling without both `aws_profile` and `region` is skipped. The ambient `$AWS_PROFILE` is
  never used.
- Each account is read once. Two environments share an account when their account ID or their
  profile matches in the same region (`awsAccountKeys`). The environment's own account is skipped
  only when its own listing worked.
- The environment's own VPC and every sibling's own VPC are left out (a sibling's range is
  already counted from its YAML). Everything else is labelled
  "another network in <env>'s AWS account".
- An unreachable sibling account never fails the check. `awsNote` names it ("Couldn't reach the
  AWS account for staging — networks there were not checked"); the technical error goes to the CLI
  only. `awsChecked` and the deployed rule still describe the environment's own account.
- Accounts are read one after another inside the 15-second AWS timeout (CLI and API alike).
- An environment with no `aws_profile`/`region` yet has nothing to fail: its note says "This
  environment isn't connected to an AWS account yet" instead of "Couldn't reach AWS".

## Which environments count as siblings

`siblingVPCCIDRUses` (`app/vpc_cidr_sources.go`) reads every `*.yaml` / `*.yml` file in the
project directory with the same filter as `listEnvironmentNames()`: not `dns`, and the file
declares both `project` and `env`. It reads the files raw, never through `loadEnv`, so scanning
never migrates, backs up or rewrites another environment's file.

| YAML | Counted as |
|---|---|
| `use_default_vpc: true` | Not counted. The default VPC is not a meroku range |
| `use_default_vpc: false`, `vpc_cidr` set | That range |
| `use_default_vpc: false`, no `vpc_cidr` | `10.0.0.0/16`, the module default |
| No `use_default_vpc`, `schema_version` < 6 or absent | Default VPC, not counted (custom VPCs arrived in v6) |
| No `use_default_vpc`, `schema_version` ≥ 6 | A custom VPC |
| `vpc_cidr` that does not parse | Not counted. Its own generate reports it |

## Is the environment deployed?

`assign` writes only when it is sure the environment has no VPC yet. The answer is the report's
`deployed` field:

| Situation | `deployed` |
|---|---|
| No `account_id` and no `aws_profile` | `no`. Nothing can exist yet |
| AWS not checked (no `--aws`) | `unchecked` |
| A VPC tagged `Project`/`Environment` for this env, or named `<project>-vpc-<env>`, exists | `yes` |
| Remote state manages an `aws_vpc` | `yes` |
| Remote state not found, or it has no `aws_vpc` | `no` |
| AWS or the state could not be read | `unknown` |

`assign` refuses for `yes` and `unknown` unless you pass `--force`, and refuses for `unchecked`
with "rerun with --aws (or --force)". Every refusal prints the suggested range and the cost of
changing it.

AWS is read through one configuration pinned to the environment's own `aws_profile` and `region`,
the same pattern as `newAWSClientsForEnv` (`app/github_oidc.go`). If either is empty, the AWS
check is skipped. meroku never falls back to the ambient `$AWS_PROFILE`, which could belong to
another account and return a confident, wrong "no overlap". An AWS failure is never an error:
the report says AWS could not be reached and the offline answer stands.

## CLI

```bash
meroku vpc-cidr check   <env> [--aws]          # rules + overlap report; exit 1 on a rule error
meroku vpc-cidr suggest <env> [--aws]          # print one free range, write nothing
meroku vpc-cidr assign  <env> [--aws] [--force]
```

Flags can come before or after the environment. AWS calls time out after 15 seconds.

`assign` is idempotent. A range that is set, valid and free is kept. Otherwise it writes a free
one, and only when the environment is known not to be deployed. Before writing, it backs up the
YAML to `backup/<env>.yaml.backup_<timestamp>` (`CreateProjectBackup`).

Example with a synthetic project:

```text
$ meroku vpc-cidr check dev
Environment: dev
vpc_cidr:    10.0.0.0/16
Effective:   10.0.0.0/16

⚠️  10.0.0.0/16 overlaps env "staging" (staging.yaml) 10.0.0.0/16

Deployed: no (no AWS account is configured for this environment yet)

$ meroku vpc-cidr assign dev
dev: vpc_cidr 10.0.0.0/16 → 10.93.0.0/16
Backup: backup/dev.yaml.backup_20261102_183627
```

## TUI

- **Create environment** prints the range it picked, for example
  `Network range (vpc_cidr): 10.137.0.0/16 — picked at random, avoiding your other environments.`
- **Selecting an environment that has no account yet:** once an AWS profile is bound, meroku asks
  "Check `<account>/<region>` for existing VPCs and move this env's range if it overlaps?". Yes
  runs `assign` with AWS on. This happens before the environment is reloaded, so a range it
  writes is the one that gets used.

## Web API

`GET /api/vpc-cidr?env=<name>[&aws=1][&count=<1..10>][&candidate=<value>]`, implemented in
`app/api_vpc_cidr.go`.

- `env` must be one of `listEnvironmentNames()`. Anything else returns 400 `{"error": "..."}`.
- `count` is the number of suggestions: default 5, clamped to 1–10. A non-number returns 400.
- `aws=1` also lists VPCs and reads remote state, and lists the VPCs in every sibling
  environment's account. A failure still returns 200, with `awsChecked: false` (own account) and
  `awsNote` (any account). `deployedReason` is always a plain sentence; raw AWS and remote-state
  errors are kept for the CLI and never reach the JSON.
- With `candidate`, the server validates a value the user is typing and checks it only against
  sibling environments. **It never calls AWS for a candidate request**, even with `aws=1`, because
  the UI sends one per keystroke. `candidate.incomplete` is true while the value has no "/" yet,
  nothing after the "/", or fewer than four numbers.
- There is no write endpoint. The web client saves the whole YAML document and would overwrite a
  server-side change with the copy it already holds.
- AWS VPCs appear only as "another network in this AWS account" or "another network in <env>'s
  AWS account". No VPC ids or names reach the
  JSON, because the API is served with permissive CORS (the same exposure as `/api/ecs/network`).

The web "Network range" card (`web/src/components/VPCRangeCard.tsx`) holds the choice locally
and saves only through one "Apply this range" button. When `deployed` is anything but `no`, the
button stays disabled until "I understand" is ticked.

## Escape hatch: `allow_vpc_cidr_override`

```yaml
use_default_vpc: false
vpc_cidr: 172.20.0.0/16          # deployed before these rules existed
allow_vpc_cidr_override: true    # keep it; turn the rule checks off
```

With the override set, `meroku generate` and the report skip the rule checks for this
environment. Overlap warnings still print. The Terraform shape check still applies. No migration
adds this field: it is optional and defaults to false.

## Research summary

From the research behind this design (2026-11). Sources are listed under [Sources](#sources).

- **Comparable tools default to a fixed range and leave overlap to you.** AWS Copilot, SST,
  Pulumi awsx and terraform-aws-modules/vpc default to `10.0.0.0/16`, eksctl to `192.168.0.0/16`,
  and Control Tower Account Factory gives every account the same `172.31.0.0/16`. Heroku Private
  Spaces sets a range at creation and never lets you change it. Only IPAM-backed setups, such as
  Landing Zone Accelerator's `ipamAllocations`, prevent overlap.
- **AWS VPC IPAM** guarantees non-overlap, but private IPv4 management is Advanced Tier only, at
  $0.00027 per active IP per hour (about $0.20 per IP per month, or roughly $10/month for a small
  environment with 50 ENIs). When IPAM picks the range, it is unknown until apply, so subnets
  cannot be planned unless the range is pinned alongside the pool id.
- **Ranges to avoid:** 172.17.0.0/16 (Docker bridge; AWS says Cloud9 and SageMaker use it),
  172.17–172.31 and 192.168.0.0/16 (Docker address pools), 172.31.0.0/16 (default VPC),
  192.168.0.0/16 and low 10.0.x (home routers, VPN clients), 10.0.0.0/15 (every tool default and
  the AWS secondary-range restriction), and 198.19.0.0/16 (restricted).
- **Once ranges overlap, peering and Transit Gateway cannot connect them.** The fallbacks are
  PrivateLink (one-way TCP, about $30/month fixed), VPC Lattice, or a private NAT gateway with
  a secondary routable range. AWS recommends renumbering first.
- **A primary range cannot be changed.** Both primary and secondary ranges must be /16 to /28.
  Moving means a new VPC: an RDS subnet-group move (which reboots the database), new ECS network
  configuration, and a new ALB and target groups with a DNS cutover.

Not done here (out of scope): IPAM mode, a PrivateLink recipe, peering or Transit Gateway
resources, and a guided blue/green network migration.

## Sources

Every source below was opened and checked against the claim it supports, grouped by the research question it answers.

### 1. How comparable tools pick default CIDRs
- [CIDR and Peering for VPC and AWS Control Tower](https://docs.aws.amazon.com/controltower/latest/userguide/vpc-ct-cidr.html) — default is 172.31.0.0/16, every VPC in the OU gets the same range so peering doesn't work at first, and a new range only applies to accounts created after the change
- [How to change CIDR range of account created by Control Tower | AWS re:Post](https://repost.aws/questions/QUjMjhQqa2SvythloHd99q5Q/how-to-change-cidr-range-of-account-created-by-control-tower) — existing accounts can't be changed; suggested fix is delete, change the range, recreate
- [VpcConfig | Landing Zone Accelerator on AWS](https://awslabs.github.io/landing-zone-accelerator-on-aws/v1.9.0/typedocs/v1.5.0/classes/_aws_accelerator_config.VpcConfig.html) — each VPC takes static `cidrs` or `ipamAllocations`
- [General - Landing Zone Accelerator on AWS (Networking FAQ)](https://awslabs.github.io/landing-zone-accelerator-on-aws/v1.9.0/faq/networking/general) — vpcTemplates use IPAM so CIDRs don't conflict, and new VPCs are handed out automatically until the pool runs out
- [env init - AWS Copilot CLI](https://aws.github.io/copilot-cli/docs/commands/env-init) — 10.0.0.0/16 default, the default public and private subnets, and the `--override-vpc-cidr` flag
- [Environment - AWS Copilot CLI (manifest)](https://aws.github.io/copilot-cli/docs/manifest/environment) — `network.vpc.cidr` can't be combined with `network.vpc.id`
- [sst/platform/src/components/aws/vpc.ts (GitHub)](https://github.com/anomalyco/sst/blob/dev/platform/src/components/aws/vpc.ts) — hardcoded `cidrBlock: "10.0.0.0/16"` and `10.0.${8*i}.0/22` subnets
- [Breaking: Vpc reference · Issue #5643 · anomalyco/sst](https://github.com/anomalyco/sst/issues/5643) — users override it with `transform.vpc.cidrBlock` and their own subnet math
- [awsx.ec2.Vpc | Pulumi Registry](https://www.pulumi.com/registry/packages/awsx/api-docs/ec2/vpc/) — 10.0.0.0/16 default, and `ipv4IpamPoolId` for an IPAM CIDR
- [Provision an AWS VPC | Pulumi Tutorials](https://www.pulumi.com/dev/tutorials/aws-vpc/) — pass `cidrBlock` to override
- [terraform-aws-vpc/variables.tf (GitHub)](https://github.com/terraform-aws-modules/terraform-aws-vpc/blob/master/variables.tf) — `cidr` defaults to "10.0.0.0/16", plus `use_ipam_pool` and `ipv4_ipam_pool_id`
- [VPC Configuration - Eksctl User Guide](https://docs.aws.amazon.com/eks/latest/eksctl/vpc-configuration.html) — 192.168.0.0/16 split into /19s, and `--vpc-cidr` for peering
- [Deploying into AWS Virtual Private Clouds (VPCs) | Flightcontrol](https://www.flightcontrol.dev/docs/guides/advanced/deploying-vpc) — a new VPC per environment by default, or an existing VPC; existing environments can't switch VPCs (deploy new, move traffic, delete old)
- [porter_eks_cluster | Terraform Registry](https://registry.terraform.io/providers/porter-dev/porter/latest/docs/resources/eks_cluster) — `vpc_cidr` must be a /16 (example 10.78.0.0/16); `service_cidr` must be a /16 within 172.16.0.0/12
- [AWS VPC peering | Porter Docs](https://docs.porter.run/other/aws-vpc-peering) — Porter VPC is "typically 10.99.0.0/16", and the peered VPCs must have different CIDRs
- [Provision Postgres and Redis datastores in your own AWS cloud using Porter](https://www.porter.run/changelog/aws-datastores) — Porter handles VPC peering for datastores it provisions
- [Private Space Peering | Heroku Dev Center](https://devcenter.heroku.com/articles/private-space-peering) — default ranges a peered VPC must not overlap (10.0.0.0/16, 10.1.0.0/16, 172.17.0.0/16)
- [Working with Private Spaces | Heroku Dev Center](https://devcenter.heroku.com/articles/working-with-private-spaces) — `--cidr` / `--data-cidr` (/20 minimum), set at creation only and never changeable after; Cedar vs Fir generations
- [Private Link Connections | Render Docs](https://render.com/docs/private-link) — PrivateLink to AWS, $30/month covers up to 3 links

### 2. AWS VPC IPAM
- [What is IPAM? - Amazon VPC](https://docs.aws.amazon.com/vpc/latest/ipam/what-it-is-ipam.html) — plan, track and monitor IPs; allocate CIDRs automatically using business rules
- [How IPAM works - Amazon VPC](https://docs.aws.amazon.com/vpc/latest/ipam/how-it-works-ipam.html) — default public and private scopes; pools are contiguous CIDR ranges and can be nested
- [Integrate IPAM with accounts in an AWS Organization](https://docs.aws.amazon.com/vpc/latest/ipam/enable-integ-ipam.html) — delegated IPAM account; existing VPC CIDRs across the org are imported automatically
- [Share an IPAM pool using AWS RAM](https://docs.aws.amazon.com/vpc/latest/ipam/share-pool-ipam.html) — sharing through RAM only works with Organizations integration
- [Amazon VPC Pricing (IPAM tab)](https://aws.amazon.com/vpc/pricing/) — Free Tier is one Region and account (BYOIP, Amazon IPv6, Public IP Insights); private IPv4 is Advanced Tier only at $0.00027 per active IP per hour; only IPs on ENIs attached to resources count as active
- [terraform-aws-vpc/examples/ipam/main.tf (GitHub)](https://github.com/terraform-aws-modules/terraform-aws-vpc/blob/master/examples/ipam/main.tf) — pool on `private_default_scope_id` with `allocation_default_netmask_length`, a 10.0.0.0/8 pool CIDR, and pinning `cidr = "10.1.0.0/16"` with `ipv4_ipam_pool_id` so the range is known at plan time
- [aws_vpc | Terraform Registry](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/vpc) — `ipv4_ipam_pool_id` + `ipv4_netmask_length` with `depends_on` the pool CIDR
- [aws_vpc_ipam_pool | Terraform Registry](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/vpc_ipam_pool) — a 10.0.0.0/8 pool with a default netmask of 16 hands out /16s by default

### 3. Best practice: random vs deterministic vs registry/IPAM
- [Building a Scalable and Secure Multi-VPC AWS Network Infrastructure (AWS whitepaper)](https://docs.aws.amazon.com/whitepapers/latest/building-scalable-secure-multi-vpc-network-infrastructure/welcome.html) — hierarchical, summarized schemes; plan IPs by environment, Region or business unit; keep on-prem and cloud ranges distinct; prevent overlaps
- [Create a hierarchical, multi-Region IPAM architecture on AWS by using Terraform (Prescriptive Guidance)](https://docs.aws.amazon.com/prescriptive-guidance/latest/patterns/multi-region-ipam-architecture.html) — top-level, Regional, business unit and environment pools (10.176.0.0/12, 10.176.0.0/15), with buffer space reserved
- [VPC CIDR blocks - Amazon VPC](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-cidr-blocks.html) — don't use 172.17.0.0/16 (Cloud9, SageMaker AI); the 10.0.0.0/15 rule for secondary CIDRs; 198.19.0.0/16 restricted
- [Troubleshooting AWS Cloud9](https://docs.aws.amazon.com/cloud9/latest/user-guide/troubleshooting.html) — 172.17.0.0/16 clashes with Docker's bridge network
- [Networking overview | Docker Docs](https://docs.docker.com/engine/network/) — default address pools are 172.17–172.31 (as /16s) and 192.168.0.0/16 (as /20s)
- [Default VPC components - Amazon VPC](https://docs.aws.amazon.com/vpc/latest/userguide/default-vpc-components.html) — the default VPC uses 172.31.0.0/16
- [VPC Configuration - Eksctl User Guide](https://docs.aws.amazon.com/eks/latest/eksctl/vpc-configuration.html) — eksctl defaults to 192.168.0.0/16
- [Numbering Private Subnets | OpenVPN](https://openvpn.net/community-docs/numbering-private-subnets.html) — avoid 192.168.0.0/24 and 10.0.0.0/24 (cafe, hotel Wi-Fi); use subnets from the middle of 10.0.0.0/8

### 4. Options when CIDRs already overlap (no renumbering)
- [How VPC peering connections work](https://docs.aws.amazon.com/vpc/latest/peering/vpc-peering-basics.html) — no peering if any CIDR overlaps, even if you only plan to use the non-overlapping blocks
- [Amazon VPC attachments in AWS Transit Gateway](https://docs.aws.amazon.com/vpc/latest/tgw/tgw-vpc-attachments.html) — no routing for identical or overlapping CIDRs of attached VPCs
- [Connecting Networks with Overlapping IP Ranges (AWS blog)](https://aws.amazon.com/blogs/networking-and-content-delivery/connecting-networks-with-overlapping-ip-ranges/) — renumber first; PrivateLink double-sided NAT (single TCP port, not UDP or multi-port with affinity); secondary routable CIDR with TGW propagation off; private NAT; Lattice
- [NAT gateway use cases - Amazon VPC](https://docs.aws.amazon.com/vpc/latest/userguide/nat-gateway-scenarios.html) — private NAT in a routable subnet plus a Transit Gateway with route propagation disabled
- [VPC CIDR blocks - Amazon VPC](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-cidr-blocks.html) — a 10.0.0.0/8 VPC can add a 100.64.0.0/10 secondary
- [What is Amazon VPC Lattice?](https://docs.aws.amazon.com/vpc-lattice/latest/ug/what-is-vpc-lattice.html) — "supports overlapping CIDR"
- [Amazon VPC Lattice FAQs](https://aws.amazon.com/vpc/lattice/faqs/) — resource configurations expose TCP resources such as RDS, shareable across accounts
- [What is VPC peering? (Pricing for a VPC peering connection)](https://docs.aws.amazon.com/vpc/latest/peering/what-is-vpc-peering.html) — free to create; traffic within one AZ is free
- [Amazon VPC Pricing (VPC Peering and NAT Gateway tabs)](https://aws.amazon.com/vpc/pricing/) — cross-AZ peering $0.01/GB in each direction; NAT gateway $0.045/hr and $0.045/GB
- [Demystifying Amazon VPC peering charges (AWS blog)](https://aws.amazon.com/blogs/networking-and-content-delivery/demystifying-amazon-vpc-peering-charges/) — how cross-AZ peering is billed in each direction
- [AWS Transit Gateway pricing](https://aws.amazon.com/transit-gateway/pricing/) — $0.05 per VPC attachment-hour, $0.02/GB
- [AWS PrivateLink FAQs](https://aws.amazon.com/privatelink/faqs/) — interface endpoints billed per hour in each AZ
- [AWS PrivateLink Pricing](https://aws.amazon.com/privatelink/pricing/) — $0.01 per endpoint per AZ-hour; $0.01/GB for the first PB
- [Comparing the Costs of Common Network Architecture Patterns with Amazon VPC Lattice | AWS re:Post](https://repost.aws/articles/AR9Tt9m6kKR6mF5Ohj5K-3Og/comparing-the-costs-of-common-network-architecture-patterns-with-amazon-vpc-lattice) — NLB $0.0225/hr (also TGW $0.05/hr and PrivateLink $0.01/hr)
- [Amazon VPC Lattice Pricing](https://aws.amazon.com/vpc/lattice/pricing/) — $0.025 per service-hour, $0.025/GB, first 300k requests/hr free, then $0.10/hr per 1M

### 5. Can the primary CIDR change? Limits and migration
- [VPC CIDR blocks - Amazon VPC](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-cidr-blocks.html) — can't disassociate the primary CIDR or resize any block; blocks must be /16 to /28
- [Troubleshooting AWS Cloud9](https://docs.aws.amazon.com/cloud9/latest/user-guide/troubleshooting.html) — "You can't change the IP address range of an existing VPC or subnet."
- [Amazon VPC quotas](https://docs.aws.amazon.com/vpc/latest/userguide/amazon-vpc-limits.html) — 5 IPv4 CIDR blocks per VPC, up to 50, primary counts; 5 VPCs per Region, adjustable
- [Updating the VPC for a DB instance - Amazon RDS](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_VPC.VPC2VPC.html) — move via subnet group; Multi-AZ and read replicas must be removed first, then added back
- [Migrate an Amazon RDS DB instance to another VPC or account (Prescriptive Guidance)](https://docs.aws.amazon.com/prescriptive-guidance/latest/patterns/migrate-an-amazon-rds-db-instance-to-another-vpc-or-account.html) — a VPC change reboots the DB; do it at low-traffic times
- [UpdateService - Amazon ECS API Reference](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_UpdateService.html) — `networkConfiguration` "triggers a new service deployment"
- [aws_lb_target_group | Terraform Registry](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lb_target_group) — `vpc_id` forces a new resource
- [Deploying into AWS Virtual Private Clouds (VPCs) | Flightcontrol](https://www.flightcontrol.dev/docs/guides/advanced/deploying-vpc) — deploy a new environment, route traffic to it, delete the old one

## Code map

| File | Layer | Contents |
|---|---|---|
| `app/vpc_cidr.go` | Domain (pure) | Rules, pool, picker, overlap warnings |
| `app/vpc_cidr_sources.go` | Infrastructure | Sibling YAML scan, EC2 VPC lister, pinned AWS config, own-VPC split |
| `app/vpc_cidr_service.go` | Application | Generate-time checks, `inspectVPCCIDR`, `assignVPCCIDR`, `newEnvVPCCIDR` |
| `app/vpc_cidr_cli.go` | Presentation | `meroku vpc-cidr`, TUI prompt |
| `app/api_vpc_cidr.go` | Presentation | `GET /api/vpc-cidr` |
| `modules/vpc/variables.tf` | Terraform | Shape validation |
