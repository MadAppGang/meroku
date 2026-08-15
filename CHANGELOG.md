# Changelog

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
