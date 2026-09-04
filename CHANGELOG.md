# Changelog

## v4.6.1

The compute pool preconditions now have a test that can fail. Their messages
moved into `modules/compute_pool_check`, a module with no provider, so
`terraform test` plans them in CI with no credentials and no network. Closes #24
and #25.

### Why the messages had no test

v4.4.1 fixed a null interpolated into the compute pool error message. That bug
shipped in v4.2.0 and survived v4.3.0, v4.3.1, v4.3.2 and v4.4.0 before a user
hit it, and CI passed on every one of those releases.

CI gives `modules/workloads` three steps: `fmt`, `init`, `validate`. Validate
does not evaluate a precondition's `error_message` at all, so it reported
success on the exact configuration `plan` rejected. Nothing else in the
repository plans that module, and nothing can: it reads eight remote data
sources (`aws_caller_identity`, `aws_region`, `aws_vpc`, `aws_lb`,
`aws_ssm_parameter`, `aws_ssm_parameters_by_path` twice,
`aws_organizations_organization`, `aws_iam_openid_connect_provider`). No
provider stub gets a plan past those, so a CI plan needs real credentials.

### What changed

`modules/compute_pool_check` decides whether a workload's pool is usable and
renders the sentence shown when it is not. It creates nothing and declares no
provider, the same property that makes `modules/naming` testable.

`aws_ecs_service.services` and `aws_ecs_service.backend` now read their
precondition from it:

```hcl
precondition {
  condition     = module.service_pool_check.valid[each.key]
  error_message = module.service_pool_check.message[each.key]
}
```

The message is rendered for every workload, the valid ones included, because
that is what Terraform does with an `error_message` and therefore what the test
has to exercise. Reintroducing the v4.2.0 bug in the module now fails
`terraform test` with the same diagnostic that reached the user:

```
Error: Invalid template interpolation value
  on main.tf line 28, in locals:
    │ w.pool is null
The expression result is null. Cannot include a null value in a string template.
```

Verified by making that edit and watching the suite fail, then reverting it.

### What the new gate caught immediately

The first CI run on this change failed, on a bug the local machine could not
see:

```
Error: Invalid function argument
  on main.tf line 12, in locals:
  12:   valid = { ... k => w.pool == null || contains(var.pool_names, w.pool) }
    │ w.pool is null
Invalid value for "value" parameter: argument must not be null.
```

CI pins Terraform 1.9.8, which evaluates both operands of `||`. Terraform 1.16
short-circuits and never calls `contains` with the null, so the same expression
passes locally and fails on the version the project actually supports
(`required_version = ">= 1.2.6"`).

The preconditions this module replaces carried that identical shape, so a
Fargate service on an older Terraform failed on `contains` rather than on the
message. Both forms are gone: `valid` is now a conditional, and Terraform
evaluates only the branch a conditional takes.

### The bare period

With no pools defined, the message used to offer an empty list and stop:

```
... or point the service at one of these: .
```

That is the likeliest state for anyone reading it, because they set
`runtime: ec2` and never wrote a `compute` block. It now says what is true:

```
... This project defines no compute pools, so a service can only use runtime
"fargate" until one is added under compute.pools.
```

The text for a project that does have pools is byte-identical to before, asserted
whole in the test rather than by substring.

## v4.6.0

Two meroku projects in one AWS account could claim the same GitHub OIDC subjects,
and nothing said so. Both applies succeed, AWS raises nothing, and a workflow in
one repository can assume both projects' roles. This release detects that, and
fixes the default that was causing it.

### The default granted your account to somebody else

`workload.github_oidc_subjects` becomes the `sub` condition on a project's GitHub
Actions role. It is the only thing deciding which repositories can assume that
role, and that role carries `iam:PassRole` over the project's task roles, ECR push
and `ecs:UpdateService`.

IAM's `StringLike` `*` is not delimiter-anchored — it crosses `:` and `/` freely.
So the shipped default was not merely broad:

```
repo:MadAppGang/*
```

A GitHub OIDC subject reads `repo:<org>/<repo>:<ref>`, so that pattern matches every
token issued to a workflow in **MadAppGang's own repositories**. For anyone who is
not MadAppGang, the default granted a third-party organisation the ability to assume
their deploy role in their own AWS account. The same value sat in
`modules/workloads/variables.tf` as a variable default, reachable by any direct
consumer of the module who omitted it.

New projects are now seeded with an obvious placeholder:

```
repo:YOUR-GITHUB-ORG/YOUR-REPO:ref:refs/heads/main
```

It must be a valid policy — an empty list renders `"sub": []`, which AWS rejects at
apply with `MalformedPolicyDocument` — while matching no real token and plainly
needing an edit. `enable_github_oidc` is off by default, so nobody meets it
unintentionally.

### Detecting an overlap that only AWS can see

A YAML-only check cannot find this. Overlap between one project's `dev.yaml` and
`prod.yaml` is the normal, correct case, and the conflicting project lives in a
separate checkout meroku never reads. So the scan reads IAM.

It lists every role in the account, identifies GitHub Actions roles by their trust
policy rather than by name, and tests whether any foreign role's subjects
*intersect* this project's — `repo:acme/api:*` and
`repo:acme/api:ref:refs/heads/main` do overlap, and exact matching would say
nothing. A confirmed conflict is reported with a **witness**: a concrete `sub` both
patterns accept, which turns "these might overlap" into "a token with this exact
claim assumes both roles".

Roles belonging to this project's own environments are excluded, because dev and
prod sharing a repository is normal — and the response says which roles it excluded
and that it deliberately did not look there.

### Three tiers, ranked by how much room there is for intent

| State | CLI |
|---|---|
| Your role has no `sub` condition, so it trusts all of GitHub | hard block, no prompt |
| Your subject grants an entire organisation | confirm; abort when non-interactive |
| Another project claims an overlapping subject | confirm; abort when non-interactive |

Only the first admits no legitimate reading, so only the first refuses outright. A
cross-project overlap can be deliberate and meroku cannot know whose intent is
right, so it asks. Non-interactive runs always abort: a CI job that auto-continues
past a confirmed conflict is the same as having no check.

### "No conflicts" never means "the read failed"

The worst defect this feature could ship is an empty conflicts array rendered as
reassurance. So `checked` is *derived*, not set by hand: it is true only when the
asserted account was scanned to completion with every candidate role evaluated or
safely excluded, and a test asserts it cannot be true while any scan-incomplete
condition is recorded.

AccessDenied, a throttle, a timeout, an unparseable trust policy, a truncated
pagination walk that returns no marker, and a role restricted by GitHub claims other
than `sub` all produce "could not verify" rather than silence. The scan also asserts
via `sts:GetCallerIdentity` that it read the account the environment names, and
refuses outright when `account_id` is unset — because an empty AWS profile is a
no-op, and without the assertion the answer would be confident and about the wrong
account.

### Breaking, for direct module consumers only

`modules/workloads` no longer defaults `github_subjects`. Terraform now asks with
"No value for required variable" instead of answering with a grant.

Configs generated by meroku are unaffected: `env/main.hbs` has emitted the key since
2024-08-29, and that is now pinned by a test, because a required variable with no
default would turn any future conditional into an unplannable config.

### Existing configs are refused, not rewritten

A config still carrying `repo:MadAppGang/*` is detected and blocked at deploy, with
no migration. Rewriting it would silently change a deployed IAM trust boundary and
could break a working pipeline at the next apply. meroku reports and refuses; the
narrowing is yours to make.

### Also

`.gitignore` excluded AI session artifacts as `ai_docs/sessions/` while the
directory in use is `ai-docs/sessions/`, so the rule never matched and session
scratch has been committed to this public repo before. Both spellings are now
listed.

## v4.5.0

A second meroku project in one AWS account can finally enable GitHub Actions.
Until now the first project into an account took the GitHub OIDC provider and
every later one failed its apply, with no setting to do anything about it.

### Why one project locked out the rest

AWS keys an IAM OIDC provider on its issuer URL, and the ARN embeds that URL as
its resource path:

```
arn:aws:iam::000000000000:oidc-provider/token.actions.githubusercontent.com
```

Nothing in that name varies per project, and the API reference is explicit:
"You cannot register the same provider multiple times in a single AWS account."

`modules/workloads/github.tf` created it whenever `enable_github_oidc` was set,
so the second project's apply stopped at:

```
Error: creating IAM OIDC Provider: EntityAlreadyExists: Provider with url
https://token.actions.githubusercontent.com already exists.
```

The IAM *role* was never part of the problem. It is already named per project,
and both its trust policy and its `iam:PassRole` scope are per project. The
provider is only a trust anchor — it says the account accepts tokens signed by
GitHub, and carries no authorization of its own. Two projects sharing one give
up nothing.

### The setting

`workload.github_oidc_create_provider`, schema v28, default `true`. Existing
projects see no plan diff. Set it to `false` and the project builds its role
against the provider another project already owns.

### The part that needed care

The obvious rule — *if the provider exists, do not create it* — is destructive.
Applied to the project that owns the provider, `count` falls from 1 to 0, and a
`count` of 1→0 destroys the real resource. The owner would delete the provider
every project in the account depends on.

So the question is not whether the provider exists. It is whether **this
environment's state owns it**:

| State owns it | Exists in account | `create_provider` |
|---|---|---|
| yes | yes | `true` — keep owning it |
| no | yes | `false` — federate against it |
| no | no | `true` — first project in the account |
| yes | no | `true` — drift, recreate it |

Terraform cannot answer that. HCL has no way to ask what is in its own state,
and the AWS provider offers no list data source for OIDC providers — only the
singular one, which fails outright when the provider is absent. meroku therefore
resolves it once, from the state and IAM together, and writes the answer into
`<env>.yaml`.

### Where you will see it

In the web UI, on the GitHub node, the moment you enable OIDC. That is where the
constraint is worth learning, rather than half an hour later in a failed apply.
It names the project that owns the provider, records the setting, and says which
backup it wrote.

The deploy path resolves it too, after the pre-flight, regenerating the terraform
when the config changed — so CLI runs and hand-edited configs are covered.

Both write through one editor that flips the key or inserts it, scoped to the
`workload` block and backing the file up first. As with `domain.enabled`, the
edit is deliberately a line rewrite rather than a marshal round-trip: re-encoding
the document would drop every key the Go type does not model, and `<env>.yaml`
is the one file in a project that cannot be regenerated.

Neither AWS read is trusted blindly, and the two ways they fail are kept apart.
`NoSuchEntity` is an answer — there is no provider — and resolves to "create it".
`AccessDenied` is a refusal to answer, and writes nothing at all. Treating the
second like the first would either create a duplicate or tell the owning project
to stop creating a provider it owns.

### `thumbprint_list` is gone

Two SHA-1 thumbprints were pinned in `github.tf`. AWS validates this issuer's
JWKS endpoint against its own trusted root certificate authorities and falls
back to thumbprints only when the identity provider's certificate is signed by
some other CA — which GitHub's is not. They were read by nothing, and would have
rotted the next time GitHub rotated certificates.

The attribute is `Optional` and `Computed`, so removing it registers no change;
the terraform-provider-aws source says so in as many words. To be precise about
what that means: providers already deployed keep the thumbprints stored in AWS.
This removes the pins from configuration, not from your account.

### Known gap

Two projects can each claim the same repository in `github_oidc_subjects`. AWS
allows it, so one repository's workflows could assume roles in both. meroku
cannot see it today: overlap between `dev.yaml` and `prod.yaml` is the ordinary
case, and a genuinely conflicting project lives in a checkout meroku never
reads. Detecting it means reading the account's IAM roles, which is its own
change.

## v4.4.1

A Fargate service in `services:` no longer breaks `terraform plan`. The
precondition added in v4.2.0 rendered a `null` into its own error message, so
the check written to explain a bad `compute_pool` became the thing that failed,
on configurations that had nothing wrong with them.

### The error

```
Error: Invalid template interpolation value
  on modules/workloads/services.tf line 282, in resource "aws_ecs_service" "services":
 282:       error_message = "Service \"${each.key}\" sets runtime \"ec2\" with compute pool
    ├────────────────
    │ each.key is "magento-bridge"
    │ local.service_pools is object with 1 attribute "magento-bridge"

The expression result is null. Cannot include a null value in a string template.
```

The condition on that precondition **passes**. `local.service_pools` maps every
non-EC2 service to `null` on purpose, because `null` is what `launch_type`,
`assign_public_ip` and the placement strategy read to mean "Fargate". So
`local.service_pools[each.key] == null` is true and the rule holds.

Terraform renders `error_message` before it tests the condition, and for every
instance of the resource. A rule that passes therefore still fails the plan if
its message interpolates a value that is legally null on the happy path.

### Who it hit

Every project with at least one service in `services:` on Fargate, from v4.2.0
(2026-08-19) onwards. Fargate is the default runtime, so the plan died before it
reached the first resource.

### The fix

The pool name reaches the message through a map that has no nulls in it:

```hcl
service_pool_names = { for k, v in local.service_pools : k => v == null ? "" : v }
```

The condition is untouched. An `ec2` service naming a pool that is missing or
disabled still fails with the same sentence, verified on a reduced case that
plans clean for a null pool and fails as intended for a bad one.

`terraform validate` does not catch this class of bug. It reported success on
the case `plan` rejected, and `modules/workloads` cannot be planned in CI
without AWS credentials, so there is no test to add here. The map is total by
construction instead, which is the same treatment every other pool read in
`ec2_capacity.tf` already had.

`modules/workloads/backend.tf` carries the same precondition and was never
affected: `var.backend_compute_pool` is `optional(string, "")` and cannot be
null.

## v4.4.0

Every AWS resource name with a length cap now goes through one algorithm
instead of being interpolated at the resource. Nothing already deployed is
renamed: a name that fits today comes back byte-identical.

### The failure, and what was actually behind it

A deploy failed on:

```
Error: "name" cannot be longer than 32 characters
  with module.workloads.aws_lb_target_group.services["..."],
  on modules/workloads/services.tf line 15
  15:  name = "${var.project}-service-${each.key}-tg-${var.env}"
```

The words `service` and `tg` spend 13 of the 32 characters AWS allows. With a
nine-character project and `dev`, seven were left for the service name.

That template was not unusual. A sweep of the module tree found 141 name sites
across 18 modules, every one of them assembling
`"${var.project}-<decoration>-${var.env}"` by hand with no length guard and no
code in common. Two break with a nine-character project; eleven break with a
nineteen-character project, `production` and a twenty-three character service
name; seventeen at thirty. The target group was simply the first to cross a
line, because 32 is the tightest cap AWS has — and the error names a line of
Terraform rather than the service that is too long.

### One cascade, first form that fits

`modules/naming` takes a map of requests and returns names:

| # | Form | Example |
|---|------|---------|
| 1 | `legacy`, verbatim | `myapp-service-api-tg-dev` |
| 2 | `project` + `parts` + `env` | `myapp-orders-sync-dev` |
| 3 | `parts` + `md5:8` | `payments-4f2a9c1e` |

The ordering is the whole design. The identity is the only field that tells two
of these apart in a console listing — project and env are the same for every
resource in the environment and are already on the tags — so it is the last
thing given up, never the first. Form 2 drops `service` and `tg`, which identify
nothing, and buys back eleven characters at no cost.

Form 1 exists only to protect what is already running. Most AWS names are
ForceNew, and a target group a listener rule still references cannot be deleted
at all, so a rename is a destroy-and-recreate whose destroy blocks. Passing a
resource's current name as `legacy` is what keeps this release from moving any
plan. `legacy_names_are_untouched` in the module's test suite asserts exactly
that: every request with a legacy name that fits must come back on form 1.

Applied to the 41 sites capped at 80 characters or less, across ten modules.
Names capped at 128 or above — IAM policies, security groups, log groups, task
definitions, SSM parameters — stay as literal templates. The cutoff is 80, and
routing a name with two hundred characters of headroom through the registry
would be churn on ForceNew resources for nothing.

It also absorbs `lambda.tf`'s own truncate-and-hash for EventBridge rule names,
which was this same idea implemented a second time.

### Why it exists twice

Terraform has no user-defined functions — 1.15.8 rejects a top-level
`functions` block outright — so the only way to write the rule once for
`modules/**.tf` is a module. But `env/main.hbs` builds SQS and SNS names from
the environment YAML, and that is rendered by meroku before Terraform is ever
invoked; no Terraform module can reach it. That half lives in `app/aws_name.go`
behind an `awsName` Handlebars helper.

The two are pinned together by `app/testdata/aws_name_vectors.json`, read by the
Go tests and mirrored by `modules/naming/tests/cascade.tftest.hcl`. Change one
half without the other and a test fails.

`legacy` is supplied by the caller in both halves rather than derived, because
there is no single historical template to derive from: `env/main.hbs` renders
`project-env-identity`, every Terraform module renders `project-identity-env`,
and several put env in the middle.

### Details that are load-bearing

`suffix` — `.fifo`, an S3 bucket postfix — is counted against the budget and is
never truncated. A FIFO queue whose name loses `.fifo` is rejected by AWS.

The digest covers project, env and the full parts list, so a truncated head
never decides uniqueness and two environments sharing one AWS account cannot
collide.

`modules/workloads` asserts the module's `collisions` output in a precondition.
The cascade lets forms mix — a short resource keeps form 1 while a long one
moves to form 2 — so a service named `service-<x>-tg` can land on the name a
service named `<x>` already has. It takes a deliberately perverse name to hit,
but without the check it arrives as a `DuplicateTargetGroupName` from the AWS
API partway through an apply, naming neither service.

### CI

`terraform test` now runs against `modules/naming`, and every module that
consumes it is initialised and validated. Previously only `modules/workloads`
was validated and no Terraform test ran at all.

## v4.3.2

`terraform fmt` is now enforced across the whole repository instead of
`modules/workloads` alone, with no exemptions. Formatting only: `git diff -w`
over `modules/` is empty, so nothing semantic moved.

### What was actually blocking it

The gate's own comment said widening the scope was a separate change, and named
two blockers. Twelve files across the other modules were unformatted, which was
the expected half.

The other half was not formatting at all. `project/env/dev/main.tf` is
generated output — `meroku generate <env>` renders `env/main.hbs` into
`env/<env>/*.tf` — that had been committed by accident and then left untouched
for two major versions. By the end it was syntactically invalid:

```hcl
  backend_remote_access = 
  docker_image = ""
```

An empty boolean render, so `terraform fmt -recursive .` failed to *parse* the
repository rather than reporting formatting on it. That is what made a
repository-wide gate impossible.

Nothing referenced the file, so it is deleted, and env renders are gitignored
in both layouts (`project/env/` and `/env/*/`) so it cannot come back.
`env/main.hbs` and `env/outputs.tf` are source and stay tracked.

With the parse failure gone, one more file surfaced outside `modules/` —
`examples/custom-extensions/post/main.tf` — and is formatted here too.

## v4.3.1

Every module now declares which AWS provider major it is built for. Nothing
about a deployed environment changes: the constraint added is the one those
environments were already resolving.

### CI was validating a provider nothing runs

Only `modules/workloads` declared a provider constraint. The other seventeen
declared none, so `terraform init -backend=false` — in CI, and in any standalone
validation of a module — resolved the newest `hashicorp/aws`, currently 6.60.0,
while every generated environment pins `~> 5.0` and gets 5.100.0.

`modules/workloads/versions.tf` already named this when EC2 compute pools
landed. This applies the same fix to the rest of the repository.

### The deprecation warnings were an artifact of that

`data.aws_region.current.name` appeared deprecated in 36 places. It is not
deprecated in the provider this repository runs: `.name` is correct in 5.x and
deprecated in 6.x, and 5.x exposes no `.region` to move to. The warning came
from validating the wrong major, so the fix is the pin rather than a rewrite —
**no `aws_region` usage changed.**

`try(data.aws_region.current.region, data.aws_region.current.name)` does not
bridge the two majors. An attribute the provider schema does not define fails
at validate time rather than at evaluation, so `try()` never reaches its
fallback. Checked against 5.100.0 rather than assumed.

Every module now resolves 5.100.0 and validates with no warnings, and a
generated environment still initialises and validates end to end.

### Two modules are deliberately untouched

- `modules/workloads` keeps its narrower floor (`>= 5.34.0, < 6.0.0`), which
  exists for `aws_ecs_capacity_provider`'s `managed_draining`.
- `modules/dns-delegation` already pins `~> 5.0` inside `main.tf`, alongside the
  `configuration_aliases` its cross-account provider needs. A module may carry
  only one `required_providers` block.

Moving to provider 6.x stays open — every module validates cleanly under 6.60.0
today — but it is deliberately not this change, because `validate` passing says
nothing about whether an existing stack plans clean across a provider major.

## v4.3.0

ECS tasks can now reach the internet through a NAT Gateway instead of each
carrying its own public IPv4 address. Existing environments are unchanged: the
schema migration stamps `egress_strategy: public_ip` everywhere, which is
exactly what meroku generated before the field existed, so the next plan after
upgrading is empty.

### Why there is a choice at all

Every task in `awsvpc` mode needs outbound access to pull its image, ship logs
and call third-party APIs. The two ways of providing it are priced on different
axes, and neither one wins everywhere:

- a **public IPv4 address** costs $3.65/task/month and nothing per GB
- a **NAT Gateway** costs $32.85/month flat and nothing per task

So the cost of public IPs grows with every service you add while ignoring
traffic entirely, and the cost of a NAT ignores service count while growing
with traffic. They cross at roughly **5 services**, or **10 in production**,
which runs one NAT per Availability Zone.

| Services | Tasks | Public IPv4 | NAT (single) | NAT (per AZ) |
|---|---|---|---|---|
| 3 | 6 | **$21.90** | $35.60 | $67.95 |
| 5 | 10 | **$36.50** | $37.25 | $69.30 |
| 8 | 16 | $58.40 | **$39.45** | $71.10 |
| 20 | 40 | $146.00 | **$49.35** | $79.20 |

The crossing is gentle rather than a cliff — at 5 services the two options are
under a dollar apart — so switching late costs very little.

### egress_strategy

```yaml
egress_strategy: public_ip       # default; a public IPv4 per task
egress_strategy: nat_gateway     # private subnets behind one NAT
egress_strategy: nat_gateway_ha  # private subnets behind one NAT per AZ
```

`modules/vpc` gains private subnets, NAT Gateways and private route tables, all
count-gated so a `public_ip` environment's plan stays exactly as it is today.
Under `nat_gateway_ha` each private route table points at the NAT in its own
zone, which avoids the $0.01/GB-each-way cross-AZ transfer charge.

Switching also improves the security posture rather than trading against it:
under either NAT strategy the tasks have no public address at all.

### The free S3 gateway endpoint is now always created

Gateway endpoints, unlike interface endpoints, have no hourly charge and no
per-GB charge. Under `public_ip` it changes nothing measurable. Under a NAT it
keeps ECR image layers — the largest single component of task egress — off the
NAT entirely, avoiding $0.045/GB for no cost.

### Networking is no longer reported as free

`calculateVPCPricing` returned `$0` and `"cost": "Free"` while every task
quietly accrued $3.65/month, including tasks an autoscaler adds without anyone
deciding to. It now prices the public IPv4 addresses an environment actually
uses. The pricing endpoint also carries an `egress` advisory saying where the
environment sits relative to the switch threshold; it never blocks anything.

### Composing with EC2 compute pools

`assign_public_ip` is a variable now rather than a hardcoded `true`, and it is
composed with the capacity-pool condition from v4.2.0: a task gets an address
only if it is on Fargate **and** the strategy asks for one. ECS rejects
`ENABLED` for EC2, so both conditions have to hold.

### Switching an existing environment

- `public_ip` → `nat_gateway` **is disruptive**. The API Gateway VPC link is
  replaced rather than updated, because its subnets cannot be changed in place
  (about two minutes), and the tasks cycle. Treat it as a maintenance action.
- `nat_gateway` → `nat_gateway_ha` **is not**. It adds a NAT and repoints one
  route table; no ECS service is touched.

A NAT strategy on the default VPC is rejected at generation time — the default
VPC has only public subnets, so it would otherwise plan clean and fail
part-way through the apply.

The full cost model, its sources and the growth curves are in
`ai_docs/EGRESS_COST_MODEL.md`.

## v4.2.0

ECS can now run on EC2 as well as Fargate, chosen per service. Everything that
already exists keeps running on Fargate and plans byte-identically — the schema
migration stamps `runtime: fargate` everywhere, and the property was verified by
diffing planned values across 68 attributes rather than argued for.

### Compute pools

A pool is a launch template, an Auto Scaling group and an ECS capacity provider
under one name. Set `runtime: ec2` on a service and, with no other configuration,
meroku synthesizes a pool for you; define `compute.pools` when you need specific
instance types, a spot mix, a floor and a ceiling, an AMI family, or your own
user data.

```yaml
compute:
  pools:
    - name: general
      instance_types: [m7i-flex.large]
      capacity_type: spot_with_base
      on_demand_base: 1
      min_size: 1
      max_size: 6

workload:
  backend_runtime: ec2
  backend_compute_pool: general
```

### Why pools use `bridge` networking, not `awsvpc`

This is the decision the rest of the feature is shaped around, and it is not the
obvious one.

Under `awsvpc` on EC2, ECS attaches a *secondary* ENI for each task. AWS
auto-assign-public-IP applies only to a *primary* interface at launch, so that
task ENI has a private address and nothing else — and this VPC has no NAT
gateway and no VPC endpoints, both removed deliberately for cost.

The failure that produces is asymmetric, which is what makes it dangerous. Image
pulls, `awslogs` shipping and secret injection all travel over the *instance's*
ENI and keep working. The ALB reaches the task, the task reaches RDS, service
discovery resolves. The task starts, registers healthy and serves traffic. Only
the application's own outbound calls — S3, SES, EventBridge, SSM at runtime, any
third-party API — time out. A deploy broken in the way that matters most reports
green in every signal an operator watches.

`bridge` egresses through the instance's existing public IP, costs nothing, and
removes the ENI density ceiling that is the economic reason to run EC2 at all: an
`m7i-flex.large` reports `maxEni: 3`, so `awsvpc` caps it at two tasks while
`bridge` fills it by CPU and memory. `awsvpc` remains available per pool, gated
on an explicit `assume_egress: true`.

The cost is stated rather than hidden: bridge-mode tasks share the instance's
security group, their target groups are `target_type = "instance"`, and Cloud Map
registers SRV records because a dynamic host port cannot live in an A record.

### Instance recommendations

The compute node reads the live instance catalog, on-demand and spot prices, and
CloudWatch utilisation for the environment's running services, then suggests
instance types for a pool under one of three postures.

It is presented as a suggested starting point, not an answer, and the UI shows
what it is based on — including saying plainly when there is no CloudWatch
history, which is the case where the advice is weakest and used to look identical
to the case where it is strongest.

Two things were wrong in the first cut and are worth recording. Suitability was
never filtered, only capability: on the real 903-type catalog, `cost-first`
returned `inf1.24xlarge` — an Inferentia ML instance — for a web workload,
because AWS reports Inferentia in neither `GpuInfo` nor `AcceleratorInfo`, so no
attribute filter can see it. Eligible families are now allowlisted, so a family
AWS ships tomorrow is excluded until someone opts in rather than silently
recommended. And two of the five sub-scores turned out to be near-inverses of one
axis weighted in opposite directions — with `headroom` identically 1.0 for every
candidate when there is no CloudWatch history, meaning the single largest weight
in the default posture ranked nothing at all. They are now one term.

The weights still have not been calibrated against a production workload. What
changed is that their consequences are pinned by a behaviour matrix, so altering
one shows up as a reviewable diff instead of a silent shift.

### Tearing a pool down

`managed_draining` makes ECS install an ASG lifecycle hook with a **3600-second**
timeout. On teardown the instance parks in `Terminating:Wait` for up to an hour
while Terraform's ECS service deletion gives up at twenty minutes, and you get:

```
Error: waiting for ECS Service delete: timeout waiting for 'INACTIVE' (last state: 'DRAINING')
Error: deleting Internet Gateway: DependencyViolation: ... has some mapped public address(es)
```

Neither message names the cause. `ai_docs/EC2_COMPUTE_POOLS.md` carries the
procedure, which was executed rather than designed — scaling to zero and removing
scale-in protection are **not** sufficient on their own; completing the lifecycle
action is what releases the instance.

### Fixed along the way

- **Postgres was reachable from the internet.** The sample config shipped
  `public_access: true` against a security group allowing 5432 from `0.0.0.0/0`,
  so every environment generated from it had a database whose credentials were
  the only control. The ingress is now VPC-scoped unless public access is an
  explicit choice.

- **`.gitignore` covered `app/dev.yaml` but not the repository root.** meroku
  writes `<env>.yaml` into whatever directory it runs from, and that file carries
  an account ID, a profile name, state bucket names and notification emails — so
  running the binary from the root dropped all of it into an unignored path in a
  public repository.

- **A named service could not use a tagged image.** `services.tf` appended
  `:latest` outside the ternary, so `docker_image: nginx:stable` became
  `nginx:stable:latest` and the task died on `CannotPullImageManifestError`. The
  three sibling modules already did it correctly; only this one did not, which is
  why it survived — it fires only when a service pins a tag, and the common case
  of leaving `docker_image` empty is unaffected.

- **`backend_autoscaling_target_cpu` and `_target_memory` did nothing.** Both
  were declared, passed through the template, and never referenced; the policies
  hardcoded 70 and 75.

- **CI validated a provider nobody runs.** `modules/workloads` had no
  `versions.tf`, so `terraform init` resolved the newest `hashicorp/aws` — 6.x —
  while every generated environment pins `~> 5.0`.

- **`meroku generate` could not find `project/dev.yaml`.** The generate path
  checked only the working-directory root while the loader searched four
  locations, so the layout this repository itself uses worked in the UI and
  failed on the command line.

### Not exercised

Two behaviours are implemented and unproven: ECS managed scale-in removing an
instance as demand falls, and renaming a pool behaving as an in-place update
rather than a service replacement. Everything else described here was observed on
a live deploy and teardown against a real AWS account.

## v4.1.0

An adaptive light/dark theme across every CLI and TUI screen (#16). Released
without a changelog entry at the time; recorded here so the history does not
appear to skip a version.

## v4.0.1

Two things that were only discoverable by hitting them, and the release that
hit one of them.

### A CloudFront distribution in front of the ALB now plans

`env/main.hbs` has always emitted `domain_name = module.workloads.alb_dns_name`
for a CloudFront origin of type `"alb"`, and `modules/workloads` has never had
that output. Any config combining the two failed at plan time on "Unsupported
attribute" — invisible until someone wrote that exact pairing, which is why it
survived this long. `alb_dns_name` and `alb_zone_id` are now exported, empty when
the ALB is off since the data source they read is not created then.

### The web app is compiled in CI

Until now it was compiled nowhere until a tag was pushed: CI covered the two Go
modules and Terraform, and the release workflow ran `pnpm build`. The first
signal that the frontend no longer compiled was therefore a failed release, which
is what happened to v4.0.0 — a type error merged green and broke the release
build, so the tag had to be moved after the fix.

The check runs `pnpm build` itself rather than an equivalent, because the
equivalent is what missed it: `pnpm build` is `tsc -b && vite build`, and the
error passes `tsc --noEmit -p tsconfig.json` while failing `tsc -b`.

That type error is also fixed. A component's fallback object literal still
described `container_command` as a string after the config type became a list, so
TypeScript widened the value to a union of the two shapes and every field present
on only one side stopped resolving — reported hundreds of lines from the change
that caused it. The literal now carries the config's own element type, so a
mismatch fails on the literal rather than at each use.

## v4.0.0

The ALB was a setting that did nothing. Turning it on created a load balancer,
never told the workloads module about it, and left every request going to API
Gateway — which cannot stream, so anything long-lived was cut off mid-response
and the one documented way out was the toggle that did not work.

This release makes that toggle real, and pins the region while it is in there.
All of it was verified against a live AWS account rather than a plan: an ALB
serving HTTPS on a real hostname, a stream held open past API Gateway's ceiling,
two projects deployed side by side in one account, and each environment torn
down afterwards.

### The ALB is an ingress toggle that works in both directions

`alb.enabled: true` was only honoured when `workload.backend_alb_domain_name`
was also set, because the template emitted `enable_alb` inside a nested
conditional on both. The extra hostname is optional and always was; requiring it
as the switch is what made the flag inert. The ALB now serves `api_domain` — the
same hostname API Gateway serves — so switching ingress does not change the
public URL, and `backend_alb_domain_name` is what it claims to be, an extra host.

`alb.idle_timeout` (default 60) is the reason to switch at all. API Gateway's
30-second integration timeout is fixed and cannot be raised, so server-sent
events and other long-lived responses are truncated. Verified end to end: a
90-second stream is cut at 30.3s with a 503 on API Gateway and completes on the
ALB with the timeout raised.

Three defects stopped the ALB path from applying at all, each hidden behind the
one before it. The API Gateway integration indexed a Cloud Map service that was
only created when the ALB was off, so the ALB path died at plan time on `Invalid
index`. `modules/workloads` referenced an `aws_security_group.alb` that does not
exist in it — the ALB's group lives in `modules/alb` — and it is now passed in.
Hostnames were hand-derived as `"${env}.${domain}"`, which ignores
`add_env_domain_prefix` and puts records outside the zone and its wildcard
certificate; `modules/domain` now exports the resolved name.

Cloud Map registration is no longer conditional, and that one is subtle. AWS
silently ignores removal of `serviceRegistries` in `UpdateService`: the call
succeeds, the task stays registered, and deleting the Cloud Map service then
fails with `ResourceInUse`. Gating it on `enable_alb` therefore made the switch a
dead end in the direction people would actually take it. A task can sit in Cloud
Map and an ALB target group at once — confirmed on a live service holding both —
so the registration stays and `enable_alb` decides only who routes.

### Two ordering bugs the plan could not see

Both surfaced only once the path could reach AWS, and both were measured rather
than reasoned about.

The first apply of an ALB environment always failed. ECS rejects a service whose
target group has no listener attached, and the listener waits on certificate
validation for minutes while the service starts immediately. The service
referenced the target group but nothing referenced the listener, so it lost the
race every time and a second apply always succeeded — the signature of a missing
edge rather than a broken config.

Disabling the ALB hung for around fourteen minutes and then failed. The backend's
security group carries a rule referencing the ALB's, so disabling it is an update
of one resource and a destroy of another, and Terraform does not reliably order
an update of a dependent before the destroy of its dependency. It destroyed the
group first and AWS refused with `DependencyViolation` while the rule still
pointed at it. Measured at 13m45s and 9m49s, each unstuck only by revoking the
rule by hand. The ALB's security group is now kept in both modes so nothing is
destroyed on a toggle and there is no ordering to get wrong; an empty group is
free and admits nothing. The same toggle now completes in 1m01s.

### An ALB with no certificate is refused before anything is built

`alb.enabled: true` with `domain.enabled: false` planned perfectly and then
failed part-way through the apply with "A certificate must be specified for HTTPS
listeners", after the VPC, the cluster, the load balancer and roughly 130 other
resources already existed. It plans because `certificate_arn` is an empty string,
which is valid HCL; only AWS knows it is not a valid listener. Generation now
refuses, naming both keys and both ways out. An HTTP-only fallback is
deliberately not offered — the module's port 80 listener is a redirect to 443, so
that path terminates TLS by design.

### The region in the config is the region you get

`region` was authoritative for the S3 backend and nothing else. The default
provider pinned no region, so resources took theirs from `AWS_REGION` or the
active profile: one value with two sources, and a shell pointed elsewhere put the
state in one region and the infrastructure in another with nothing to say so.

The provider is now pinned from the config. That does not create the split, it
reveals it — a stack already running mismatched will plan a move — so generation
warns when the shell disagrees, naming both regions and stating that the move is
a destroy and recreate rather than a migration. A warning and not an error,
because on a stack that does not exist yet there is nothing to relocate.

### Scheduled tasks

`container_command` is a `list(string)` in Terraform and was a scalar string in
the model, rendered raw into HCL. A scalar therefore reached `main.tf` unquoted,
so the only way to get a valid list was to type the HCL yourself, and real
configs hold a mix of bare commands and hand-written JSON arrays. Migration v25
converts both. The same defect on event-processor tasks rendered a `[]string` as
`npmruncron` — Terraform's own concatenation of a slice, silently unparseable.

Also adds `timezone`, so a cron set for 09:00 stays at 09:00 local across
daylight saving; an optional `dlq_arn` with the `sqs:SendMessage` grant scoped to
that one queue; and `max_retry_attempts`. Retries are opt-in through a `dynamic`
block rather than a variable with a default, because AWS's own default is 185 and
any default of ours would rewrite that number for every existing task on the next
apply. Absent leaves 185, `0` means never retry, and both survive to the live
schedule.

### Amplify cross-account DNS

`modules/amplify` already had `manage_dns_records` and the Route53 records it
gates; nothing set it. Amplify writes its own records when the zone is in the
same account, so this is for the cross-account and externally-managed case only.

### Breaking

`alb.enabled` now takes effect on its own. An environment that set it while
relying on `backend_alb_domain_name` being absent to keep API Gateway will move
to the ALB on the next apply. The public URL does not change, and the switch is a
normal ECS rolling deployment.

`scheduled_tasks[].container_command` becomes a list. Migration v25 rewrites
existing configs on load; a config written by hand after this release must use
list syntax.

Adding `count` to the ALB module moves two resources to `[0]`. Terraform
migrates that itself — the plan reports "has moved to" with nothing to add,
change or destroy — so no `state mv` is needed. Confirmed live in both
directions.

## v3.27.0

Two things the UI told you that were not true: a delete on the map that deleted
nothing, and an apply progress bar that counted past its own total.

### Deleting a node on the map deletes the resource

Selecting a service, scheduled task, event task, Amplify app or CloudFront
distribution on the canvas and pressing Delete removed it from the map and left
the YAML untouched, so the resource came back on the next reload. Deleting the
same resource from the properties panel's trash button always worked, which is
what made this hard to see: two delete affordances, one of them a no-op.

There were two delete paths and only one of them reached the config. The trash
button called `handleDeleteNode`, which rewrites the config and saves it.
React Flow's own delete key produced a `remove` node change that
`handleNodesChange` forwarded straight to `applyNodeChanges`; that handler only
ever inspected changes for position updates, so the node left React Flow's
internal list and nothing was written.

The nodes on this canvas are a projection of the config — `allNodes` is a memo
over it — so applying a removal to local node state makes the canvas disagree
with its own source of truth. Removals now go to the same handler the trash
button uses and are *not* applied locally: the node leaves the map when the save
lands and the memo re-derives it, and it stays put if you cancel. The
confirmation moved into `handleDeleteNode`, so both entry points ask the same
question exactly once.

### Apply progress no longer overruns its total

A deploy of 68 planned resources finished reporting `100% 102/68`. Terraform's
own summary for the same run was `52 added, 13 changed, 37 destroyed` — 102
operations, of which 34 were replacements counted once in the denominator and
twice in the numerator.

`calculateStatistics` already counted a replacement as two operations. Nothing
ever reached that code: `groupResourceChanges` classified changes with
`switch Actions[0]` and looked for a literal `"replace"` action, which terraform
never emits. A replacement arrives as a two-action pair — `["delete","create"]`,
or `["create","delete"]` under `create_before_destroy` — so every one of them
was filed as a plain delete or create and counted once, while the apply reported
a completion hook for each half.

Classification now matches on the pair, both orders. The plan header folds
replacements into both columns the way terraform's summary does, so what it
predicts before the run matches the `Apply complete!` line after it, and the
three columns sum to the progress denominator.

The existing tests hand-built the grouped structs, which is how the dead branch
survived — they exercised the counting and never the classification that fills
it. The new ones start from raw plan actions.

## v3.26.0

Two defects that between them made a fresh environment undeployable: one that
stopped the plan outright, and one that made a recreated hosted zone unusable
for up to two days without saying so.

### A first deploy with services can be planned again

Any environment with at least one service could not be planned from scratch:

```
Error: Invalid count argument
  on modules/workloads/lambda.tf line 511, in resource "aws_cloudwatch_event_rule" "ci_ecr_push":
  511:   count = length(local.ci_ecr_repos) > 0 ? 1 : 0
```

The line named is not the problem. `ecr.tf` resolved each service's repository
from `aws_ecr_repository.services[...].repository_url`, and that attribute is
Computed — unknown on the plan that first creates the repository. The unknown
travelled through a `for` expression whose `if` clause filtered on it, which
makes the resulting list's *length* unknown, and that length is what the ECR
event rule counts on. Terraform refuses a count it cannot resolve, so the plan
failed before anything was created. Once the repositories existed the URL was
known and the error disappeared, which is why it only ever hit new environments.

`ecr.tf` now resolves the bare repository names alongside the URLs, from the
same per-mode branching, and the CI Lambda's repository set reads those. A name
is set from configuration, so it is known on the same plan — and a bare name is
what an ECR event carries anyway. A boundary test fails if either half of that
path goes back to reading the URL.

### A recreated hosted zone no longer strands resolvers for two days

Route53 assigns a fresh, random nameserver set to every hosted zone it creates,
and publishes the zone's own apex NS records with a TTL of 172800 — two days. A
resolver caches that RRset from the child's authoritative answer rather than
from the parent's referral, so the 300s TTL on the delegation record meroku
writes into the parent governs only resolvers that have nothing cached.

Destroy an environment and apply it again — which any dev cycle does — and every
resolver that looked the name up beforehand keeps querying the previous zone's
nameservers. Those servers no longer host the zone, so they answer REFUSED and
the name returns SERVFAIL rather than merely resolving to something stale. It
stays that way for up to two days regardless of how correct the new delegation
is, and certificate validation asks through exactly such a resolver.

`modules/domain` and `modules/dns-delegation` now republish each zone's apex NS
records at a 300s TTL, matching the delegation written into the parent, so both
halves of the referral expire together. The values are unchanged — only the TTL.
The record is created in phase 1 alongside the zone, because the propagate step
consults public resolvers minutes before phase 2 exists and would otherwise be
seeding them with the two-day copy.

### The propagate screen distinguishes a stale delegation from a slow one

The DNS-over-HTTPS check reduced every response to "does this resolver see our
nameservers", which folded three different situations into one. A resolver with
a cold cache, one holding a negative answer, and one still serving a previous
incarnation of the zone all rendered identically as `checking…`. Only the first
two clear in minutes.

The rcode was already being parsed and then discarded. It is now kept, and a
resolver reports one of three states: resolved, not yet, or **STALE** — either
SERVFAIL, or an answer naming nameservers that are not ours. Stale resolvers get
a red badge that persists through re-checks instead of a spinner implying
progress, and the panel says what is actually happening rather than describing a
cache that is about to expire.

Stale resolvers do not count as agreement and do not shorten the settle window;
the existing three-minute cap still releases the deploy. The done panel now
names the condition, because a resolver on an older delegation is the likeliest
explanation if the apply then stalls on certificate validation.

### The settle window shows its deadline

Partial agreement holds for up to three minutes so the certificate request does
not race a resolver with a stale negative cache. The only timer on screen was
"next check in 10s", which resets forever, so a bounded wait doing exactly its
job was indistinguishable from a hang — and was reported as one. The screen now
shows how long is left before it continues without the remaining resolvers.

## v3.25.0

Same code as v3.24.0, released from `main`.

v3.24.0 was tagged on the feature branch before it merged. The binaries it
published are correct and identical to these; this release exists so the
published version is one anybody can reach from `main`.

Everything below applies to both.

## v3.24.0

The CI/CD Lambda is rewritten. Its main path — redeploy the backend when a new
image is pushed — had never worked, and two authentication bypasses in the
AppSync authorizer are closed. A fresh checkout of a deployed project now
connects itself instead of reporting that nothing is deployed.

### Choosing how AppSync authenticates

The API hardcoded a Lambda authorizer and attached an API key beside it,
unconditionally. Anyone with that key skipped JWT verification altogether, which
made the authorizer decorative. Authorization is now chosen per environment:

```yaml
pubsub_appsync:
  enabled: true
  auth_mode: cognito        # or: oidc, lambda
  api_key_enabled: false
```

- **`cognito`** — validated by AppSync against the user pool meroku already
  creates. `modules/cognito` gained the `outputs.tf` it never had, which is why
  the pool could not be wired before.
- **`oidc`** — validated by AppSync against a configured issuer. `oidc_client_id`
  is matched against the token's `aud` (or `azp`) claim, and accepts several
  client ids separated by `|`.
- **`lambda`** — the bundled authorizer. Needed only when you must check claims
  beyond issuer and audience; it now supports required-claim policies, failing
  closed with a denial reason distinct from a bad signature or a JWKS outage.

The first two are verified by AWS, so no Lambda is built, deployed or invoked.

Cognito mode previously had **no audience check at all** — it accepted a token
minted for any app client in the pool, and meroku creates three.
`cognito_app_id_client_regex` now filters them; leaving it unset still accepts
all of them, and the docs say so.

The API key defaults off. Existing AppSync environments keep theirs — the
migration writes `api_key_enabled: true` where AppSync is already enabled, since
withdrawing a live credential on upgrade would break its clients.

Note for single-mode OIDC APIs: AppSync skips comparing the `iss` claim against
the configured issuer unless a second authorization mode is present. Signature
verification against that issuer's JWKS still applies. Use `lambda` mode if you
need `iss` asserted.

### Two defects found by rendering a config with Cognito enabled

- `dashboard_callback_urls` was tagged `dashboard_callback_ur_ls` in the Go
  struct — an acronym split by a camelCase conversion. Because the struct both
  reads and writes the file, every load dropped configured URLs and every save
  wrote them back misspelt. Fixed at the tag, with a migration repairing configs
  on disk.
- The `array` template helper panicked on a missing key. All twenty of its call
  sites read optional fields, so any config omitting one crashed generation with
  a Go stack trace. An absent list now renders `[]`.

### Syncing a checkout to what is deployed

`env/` is generated and gitignored, so a fresh clone never has one. meroku used
to read that absence as "nothing is deployed" — while the resources sat in the
S3 state, untouched — and every terraform command then failed with `Backend
initialization required`.

It now asks the only thing that knows. When `env/<env>/.terraform` is missing,
meroku reads the state backend named in `<env>.yaml`: resources there mean the
environment is deployed, and it offers to regenerate `env/<env>/`, connect it,
and run a plan so you can see whether your checkout matches what is running. No
state means a genuinely new project, and it stays quiet.

    meroku sync [env]

does the same on demand and reports whatever it finds. The automatic path asks
before writing anything, and a refusal is not remembered — it asks again next
time. `meroku sync` never asks, since running it is the consent, and neither
does a non-interactive shell.

Nothing on this path applies, destroys, or migrates state.

Also fixed: `meroku generate` rendered Terraform through a loader that never ran
migrations, so a config two schema versions behind produced output with defaults
silently substituted for every missing field. And environment discovery matched
any root YAML by filename, offering `Taskfile` as a deployable environment; an
environment is now identified by declaring both `project` and `env`.

### Upgrade actions

Do these before deploying.

1. **Install Go 1.22+ on whatever machine runs `terraform apply`.** The Lambda is
   now built at apply time instead of shipping a prebuilt binary. The apply fails
   with an actionable message if `go` is missing; nothing is substituted for it.

2. **State how AppSync authenticates, on every environment with it enabled.**
   There is no default, and generation refuses to proceed without it. Pick the
   mode that matches your identity provider:

   ```yaml
   # Cognito issues the tokens — AWS verifies them, no Lambda runs
   pubsub_appsync:
     enabled: true
     auth_mode: cognito
     cognito_app_id_client_regex: "1F4G9H|1J6L4B"   # which app clients; unset accepts all

   # An OIDC provider issues them — AWS verifies them, no Lambda runs
   pubsub_appsync:
     enabled: true
     auth_mode: oidc
     oidc_issuer: https://your-idp.example.com
     oidc_client_id: "your-client-id"               # matched against aud / azp

   # Anything else, or you need to check custom claims
   pubsub_appsync:
     enabled: true
     auth_mode: lambda
     jwks_uri: https://your-idp.example.com/.well-known/jwks.json
     jwt_issuer: https://your-idp.example.com/      # optional, enforced when set
     jwt_audience: your-api                          # optional, enforced when set
   ```

3. **Run `meroku migrate all`.** Schema v24. v22 adds `auto_deploy` (production
   environments get `false`; everything else keeps auto-deploying). v23 adds the
   AppSync `auth_mode` and `api_key_enabled`. v24 repairs the misspelt
   `dashboard_callback_urls` key.

4. **Expect one apply to replace the EventBridge rules.** The single CI rule
   becomes four. There is a brief window during the apply where no rule is
   attached, so deploy triggers fired in that window are lost. Apply during a
   quiet period if that matters.

### Fixed

- **Backend auto-deploy never worked.** Terraform emitted the service key
  `"backend"` while the Lambda looked up the empty string, so every backend ECR
  push failed with `service '' not found in ECS_SERVICE_MAP`. All six test files
  passed throughout, because each side tested its own assumption.

- **Two projects sharing an AWS account cross-deployed each other.** The event
  rule filtered only source and detail-type, and the repository regex ignored the
  project name, so a push to `projectA_service_api` could redeploy project B's
  `api`. Identifier resolution is now pure lookup against Terraform-emitted maps,
  so another project's repository is simply absent and no code path can act on it.

- **The wrong task-definition revision was deployed past `:9`.** ARNs were
  re-sorted as strings after AWS had already sorted them newest-first, so `:9`
  ranked above `:11`.

- **A failed Lambda build deployed a 97-byte text file.** The build error was
  discarded, and a placeholder was written so `archive_file` would not complain.
  The result deployed green and failed every invocation with
  `Runtime.InvalidEntrypoint`.

- **`terraform apply` silently rolled back CI deployments.** ECS services now
  carry `ignore_changes = [task_definition]`: Terraform owns the service's shape,
  CI owns the revision running in it.

- **Slack failures could fail a deployment**, causing EventBridge to retry and
  post duplicates. Notification can no longer fail an invocation, and permanent
  errors are no longer classified as retryable.

- **`context.Context` was accepted by every handler and then discarded** at the
  deployer boundary. It now reaches every AWS SDK call, and retries respect
  cancellation instead of sleeping past the Lambda deadline.

- **Two template conditions never resolved** — `appsync.resolvers` and
  `pubsub_appsync.appsync.auth_lambda` — so `vtl_templates_yaml` and
  `auth_lambda_path` had never reached the module. A false `{{#if}}` is
  indistinguishable from a disabled feature.

- **The AppSync authorizer had no `aws_lambda_permission`**, so AppSync could not
  invoke it at all.

### AppSync authorizer

`JWKS_URI` fell back to a hardcoded third-party endpoint when unset, and
Terraform set only `EXAMPLE_VAR` — so the fallback was always the path taken, and
whoever controlled that tenant could mint tokens every deployment accepted.
Verification was also skipped entirely when `NODE_ENV=development`, trusting a
base64-decoded payload.

Both are removed. `JWKS_URI` is required and fails closed, issuer and audience
are enforced when set, `resolverContext` values are strings as AppSync requires,
and denials carry reason codes with TTLs that distinguish an invalid token from a
JWKS outage.

### Added

- **`auto_deploy` per target** (backend, services, scheduled tasks). CI policy,
  distinct from `enabled`: whether an ECR push, SSM change or S3 env-file write
  may redeploy without anyone asking. Manual deploys ignore it. Absent means
  `true`, so projects predating the field are unaffected.

- **A boundary test spanning Terraform and Go.** A golden fixture of the
  identifiers Terraform emits, asserted against the real resolver, covering
  environment variable names as well as their contents. This is the mechanism
  that keeps the backend bug fixed; corrupting either side fails the build.

- **CI that actually runs it** (`.github/workflows/ci.yml`): build, vet, gofmt
  and `go test -race` for both Go modules, plus `terraform fmt` and `validate`.
  There was previously no workflow running any test.

### Changed

- AWS SDK v1 (end of support July 2025) → v2; Go 1.20 → 1.22.
- Hand-rolled logging → `log/slog`. Attributes are top-level rather than nested
  under `fields`, so CloudWatch Logs Insights queries need updating.
- The Lambda is built for **arm64** and packaged by Terraform.
- Manual-deploy snippets emit `project` so deploys can be scoped per project.
- Removed: `SERVICE_CONFIG` (passed but never read) and the CLI's separate amd64
  Lambda build (produced an artifact nothing deployed).
