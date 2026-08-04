# CI Lambda Rewrite — Architecture

Companion to `requirements.md` in this directory. Defect IDs (D1–D17), FR/NFR IDs, and open
questions (Q1–Q6) refer to that file.

---

## 1. Shape of the system

```
                  ┌──────────────────────── contract/contract.json ───────────────────────┐
                  │  backend_id, task_id_prefix, repo_*_fmt, ssm_*_fmt   (ONE definition)  │
                  └───────┬──────────────────────────────────────────────┬────────────────┘
             read by HCL  │                                              │  go:embed
                          ▼                                              ▼
        ┌─────────────────────────────────┐              ┌──────────────────────────────┐
        │ modules/workloads/ci_identifiers│              │ ci_lambda/contract (Go pkg)  │
        │  pure module, no providers      │              │  Format()  +  Parse()        │
        │  outputs: ecr_repo_ids,         │              │  (Parse = 2nd, independent   │
        │           ssm_prefix_ids,       │              │   derivation, used only by   │
        │           service_ids, task_ids │              │   SelfCheck and tests)       │
        └───────────────┬─────────────────┘              └───────────────┬──────────────┘
                        │ consumed by lambda.tf for BOTH                 │
                        │ the event patterns AND the env vars            │
                        ▼                                               ▼
        ┌─────────────────────────────────┐  env vars   ┌──────────────────────────────┐
        │ 4 EventBridge rules  ──────────►│────────────►│  Lambda (provided.al2)       │
        │ ecr / ecs-state / ssm / manual  │             │  cold start → SelfCheck      │
        └─────────────────────────────────┘             └──────────────┬───────────────┘
                                                                        │
                          ┌─────────────────────────────────────────────┴──────────────┐
                          ▼                             ▼                              ▼
                  handler (route)             deploy (retry policy)          slack (fire & forget)
                                                        │
                                          ┌─────────────┴──────────────┐
                                          ▼                            ▼
                                 awsecs.UpdateService       awsecs.RegisterRevisionWithImage
```

Event → identifier resolution is **lookup, never parsing**. Terraform already does this for
`ECS_SERVICE_MAP` and `S3_SERVICE_MAP`; this rewrite finishes the job for ECR and SSM, which
are the two paths that still parse and are the two paths that are broken (D1, D2, D9, D16, D17).

---

## 2. The D1 firewall — four layers

D1 exists because two sides invented the same identifier independently and nothing compared
them. Fix the cause, not the symptom.

### L1 — One definition, consumed by both sides

`modules/workloads/ci_lambda/contract/contract.json`:

```json
{
  "backend_id":          "backend",
  "task_id_prefix":      "task:",
  "repo_backend_fmt":    "%s_backend",
  "repo_service_fmt":    "%s_service_%s",
  "repo_task_fmt":       "%s_task_%s",
  "ssm_backend_fmt":     "/%s/%s/backend",
  "ssm_service_fmt":     "/%s/%s/%s",
  "ssm_task_fmt":        "/%s/%s/task/%s"
}
```

Terraform reads it with `jsondecode(file(...))` and applies it with `format()`. Go embeds it
with `//go:embed` and applies it with `fmt.Sprintf`. Same verbs, same format strings, one file.
`format()` and `Sprintf` agree on `%s`. Neither `lambda.tf` nor any `.go` file may contain the
literal `"backend"` or `"task:"` in an identifier position.

The file lives inside the Go package directory because `go:embed` cannot reach above its own
package. Terraform's path to it is `${path.module}/ci_lambda/contract/contract.json`.

### L2 — A pure Terraform module that a test can actually execute

`modules/workloads/ci_identifiers/` computes identifiers and nothing else: no providers, no
resources, no data sources. That property is what makes it evaluable in a test with
`terraform init -backend=false && terraform apply` in a temp dir, with no AWS credentials.

`lambda.tf` consumes its outputs for **both** the event patterns and the env vars, so the
identifiers the test evaluates are literally the identifiers production uses.

```hcl
# modules/workloads/ci_identifiers/main.tf
variable "project"              { type = string }
variable "env"                  { type = string }
variable "service_names"        { type = list(string), default = [] }
variable "service_repo_names"   { type = map(string), default = {} }  # name -> actual ECR repo name
variable "scheduled_task_names" { type = list(string), default = [] }

locals {
  c = jsondecode(file("${path.module}/../ci_lambda/contract/contract.json"))

  backend_id  = local.c.backend_id
  service_ids = { for n in var.service_names : n => n }
  task_ids    = { for n in var.scheduled_task_names : n => format("%s%s", local.c.task_id_prefix, n) }

  # repo name -> [identifier]; fan-out covers ecr_config use_existing / manual_repo (D9)
  repo_pairs = concat(
    [{ repo = format(local.c.repo_backend_fmt, var.project), id = local.backend_id }],
    [for n in var.service_names : {
      repo = lookup(var.service_repo_names, n, format(local.c.repo_service_fmt, var.project, n))
      id   = n
    }],
    [for n in var.scheduled_task_names : {
      repo = format(local.c.repo_task_fmt, var.project, n)
      id   = local.task_ids[n]
    }],
  )
  repos = distinct([for p in local.repo_pairs : p.repo if p.repo != ""])

  ssm_pairs = concat(
    [{ prefix = format(local.c.ssm_backend_fmt, var.env, var.project), id = local.backend_id }],
    [for n in var.service_names : {
      prefix = format(local.c.ssm_service_fmt, var.env, var.project, n), id = n
    }],
    [for n in var.scheduled_task_names : {
      prefix = format(local.c.ssm_task_fmt, var.env, var.project, n), id = local.task_ids[n]
    }],
  )
}

output "backend_id"     { value = local.backend_id }
output "service_ids"    { value = local.service_ids }   # name -> identifier
output "task_ids"       { value = local.task_ids }      # name -> identifier
output "ecr_repo_ids"   { value = { for r in local.repos : r => [for p in local.repo_pairs : p.id if p.repo == r] } }
output "ssm_prefix_ids" { value = { for p in local.ssm_pairs : p.prefix => p.id } }
```

Note `ssm_task_fmt` produces `/dev/p/task/cleanup`, which is a *longer* prefix than
`/dev/p/task` — longest-prefix matching in Go resolves the ambiguity if a service is ever
named `task`.

### L3 — Cold-start self-check (the runtime backstop)

`config.SelfCheck(spec)` runs once at init, over the maps Terraform actually shipped:

1. every value in `ECR_REPO_MAP` and `SSM_SERVICE_MAP` and every key in `S3_SERVICE_MAP`
   resolves to an entry in the merged target map;
2. every target key round-trips: `spec.ExpectedRepo(project, id)` → `spec.ParseRepo(project, repo)`
   must return `id` again — Go's *lookup* path and Go's *independent parser* must agree on data
   Terraform produced;
3. `backendID` is present as a target key.

Failure returns an error from `Initialize`, the Lambda fails to start, the first invocation
errors visibly (Q5). Under D1 this would have fired on the first deploy after the change, in
CloudWatch, instead of hiding for months.

### L4 — Boundary test that runs Terraform (the one the task asks for)

`modules/workloads/ci_lambda/contract/tfboundary_test.go`, build-tagged `tfboundary`,
`t.Skip` when `terraform` is not on `PATH`, mandatory in CI:

```go
func TestTerraformToGoIdentifierBoundary(t *testing.T) {
    // synthetic project: hyphenated service, use_existing service sharing a repo, one task
    dir := writeScratchModule(t, `
      module "ids" {
        source               = "../../ci_identifiers"
        project              = "acme"
        env                  = "dev"
        service_names        = ["api", "payment-worker", "reporting"]
        service_repo_names   = { reporting = "acme_service_api" }   # use_existing fan-out
        scheduled_task_names = ["cleanup"]
      }
      output "ids" { value = module.ids }
    `)
    out := terraformOutputJSON(t, dir)          // init -backend=false && apply && output -json

    // 1. Terraform's identifiers must equal Go's Format() derivations.
    spec := contract.Load()
    require.Equal(t, spec.BackendID(), out.BackendID)
    require.Equal(t, spec.TaskID("cleanup"), out.TaskIDs["cleanup"])
    require.Equal(t, spec.ServiceRepo("acme", "payment-worker"), repoOf(out, "payment-worker"))

    // 2. Terraform's maps must be fully resolvable by the real Go config.
    cfg := mustLoad(envFromTerraform(out))       // builds ECS_SERVICE_MAP/SCHEDULED_TASK_MAP/… from out
    require.NoError(t, cfg.SelfCheck(spec))

    // 3. Every event this project can emit must resolve.
    for repo, wantIDs := range out.ECRRepoIDs {
        require.ElementsMatch(t, wantIDs, cfg.IdentifiersForRepo(repo))
        for _, id := range wantIDs { _, ok := cfg.Target(id); require.True(t, ok, id) }
    }
    for prefix, wantID := range out.SSMPrefixIDs {
        got, ok := cfg.IdentifierForSSMPath(prefix + "/env"); require.True(t, ok)
        require.Equal(t, wantID, got)
    }
    // 4. Cross-project isolation (D2)
    _, ok := cfg.Target(mustParse(spec, "acme", "otherproj_service_api")); require.False(t, ok)
}
```

The fan-out case (`reporting` sharing `acme_service_api`) also pins D9 behaviour: one repo,
two identifiers, two deployments.

**Static guard**, cheap, same package, always runs: `TestLambdaTFContainsNoIdentifierLiterals`
reads `../../lambda.tf` and fails if it matches `"backend"\s*=` or `"task:` or
`"%s_service_%s"`, and fails if it does not reference `module.ci_identifiers`. Reintroducing
D1 by hardcoding a key breaks the build.

---

## 3. Package layout

```
modules/workloads/ci_lambda/
├── go.mod                       # go 1.22, aws-sdk-go-v2, aws-lambda-go, testify
├── main.go                      # ~50 lines: cold start, wiring, lambda.Start
├── contract/
│   ├── contract.json            # L1 single source of truth (read by HCL, embedded by Go)
│   ├── contract.go              # Spec: Format* + Parse* + Expected*
│   ├── contract_test.go
│   ├── lambdatf_guard_test.go   # L2 static guard over ../../lambda.tf
│   └── tfboundary_test.go       # L4 (build tag: tfboundary)
├── internal/
│   ├── config/{config.go,selfcheck.go,config_test.go}
│   ├── awsecs/{client.go,image.go,client_test.go,image_test.go,fake_test.go}
│   ├── slack/{client.go,templates/{success,error,info}.json.tmpl,client_test.go}
│   ├── deploy/{deployer.go,retry.go,deployer_test.go}
│   └── handler/{handler.go,ecr.go,ssm.go,s3.go,manual.go,ecsstate.go,*_test.go}
└── internal/testsupport/fixture.go   # synthetic project → env-var set, shared by all tests
```

`contract/` is deliberately **not** under `internal/`: Terraform references its JSON path, and
keeping it at the top signals that it is the cross-boundary artifact.

---

## 4. Fate of every existing file

| File | Fate |
|---|---|
| `main.go` | Rewritten. Loses `Application` struct sprawl, the `"architecture": "v2-direct-naming"` fields, and `Initialize()`'s ten log lines. Cold start builds `aws.Config` once with ctx. |
| `config/config.go` | → `internal/config/config.go`. `ServiceMapping`→`Target`, `Type`→`Kind`. `LoadFromEnv()`→`Load(getenv func(string) string)` (no `os.Setenv` in tests). Drops `DeploymentTimeoutSeconds`, `GetServiceInfo`, `ListAllServices`, `GetServiceName`, `GetTaskFamily`. Adds `ECRRepos`, `SSMPrefixes`, `SelfCheck`. |
| `config/config_test.go` | Rewritten. Existing tests key the map on `""` (`config_test.go:27,99`) — they encode D1 and must not be ported. |
| `handlers/handler.go` | → `internal/handler/`, split by source. `EventHandlerV2`→`Handler`. `extractServiceFromSSMPath` (the `(\w+)` regex, D16/D17) deleted in favour of prefix lookup. `ctx` threaded to every call (D4). Slack no longer able to fail the invocation. |
| `handlers/handler_test.go` | Rewritten. Its `testConfig()` keys `"backend"` while `TestHandleECREvent_BackendPush` asserts `""` — the test suite literally documents both halves of D1 and never compares them, because `mockDeployer` records the identifier without resolving it. New tests build config from `internal/testsupport`. |
| `deployer/deployer.go` | → `internal/deploy/deployer.go`. `DeployResult.Error` replaced by a real `error` return. Linear "exponential" backoff (`:95`) replaced. `extractServiceIdentifiers`'s `"(backend)"` hack (`:212`) deleted with the sentinel. |
| `deployer/deployer_test.go` | **Deleted, not ported.** It tests `testableDeployer`, a copy of the branching logic, not `DeployerV2` (D15). Replaced by tests that drive the real `Deployer` through an ECS fake and assert retry counts, backoff calls, and notification independence. |
| `services/ecs.go` | → `internal/awsecs/client.go` on SDK v2. `getLatestTaskDefinition` **deleted** (D3 root cause). `RegisterNewTaskDefinitionRevision`→`RegisterRevisionWithImage` with full field carry-forward (D6) and digest-safe image parsing. `DescribeService`, `ListAllServices`, `VerifyServiceExists` deleted — only `cmd/integration_test.go` used them. |
| `services/ecs_scheduled_task_test.go` | Rewritten against the fake; dry-run assertions kept; `NewECSServiceV2` no longer constructs a live client in tests. |
| `services/slack.go` | → `internal/slack/client.go`. `SendNotification(data) error` → `Notify(ctx, Message)` with no return value (NFR-4). Inline template strings → embedded files. |
| `utils/logger.go` | **Deleted.** `log/slog`. |
| `utils/service_name_extractor.go` | → `contract/contract.go` as `Spec.ParseRepo`, anchored with `regexp.QuoteMeta(project)` and `(.+)` instead of `\w+`. Demoted: no longer on the request path, used by `SelfCheck` and tests as the independent second derivation. |
| `utils/service_name_extractor_test.go` | → `contract/contract_test.go`. Its case `"backend repo returns empty string"` inverts. New cases: `otherproj_service_api` must **not** parse under project `acme`; hyphenated names; `acme_task_cleanup_extra` vs `acme_task_cleanup`. |
| `cmd/integration_test.go` | **Deleted.** A `func main()` named `_test.go`, run by `make integration-test`, requiring live AWS. Superseded by L4 plus a documented `aws lambda invoke` smoke script. |
| `slack.message.*.json.tmpl` (3) | **Deleted** from the root; equivalents recreated under `internal/slack/templates/` and embedded. The current files are dead, reference `{{.Env}}` (not a field), and emit invalid JSON (D10). |
| `Makefile` | **Deleted.** The repo standardises on Taskfile; `Makefile` and `Taskfile.yml` already disagree (`go build … main.go` vs `.`). |
| `README.md` | Rewritten. Removes the V1/V2 narrative, the false "Phase 1: pattern-based extraction is safe" claim (it is exactly what broke), the false step 6 "Wait for Stability", and the stale `bootstrap (17MB)` line. |
| `go.mod` / `go.sum` | `go 1.22`; `aws-sdk-go-v2/{config,service/ecs}`; `aws-lambda-go` → current; `testify` → current; `aws-sdk-go` v1 removed. |
| `bootstrap` | Already untracked and gitignored (`.gitignore:32`). Now produced by Terraform at apply (D12, FR-17). |

---

## 5. Signatures

### 5.1 `contract`

```go
package contract

type Spec struct { /* unexported fields from contract.json */ }

func Load() Spec                                    // sync.Once over embedded JSON; panics on malformed (build-time data)

func (s Spec) BackendID() string                    // "backend"
func (s Spec) TaskID(name string) string            // "task:" + name
func (s Spec) BackendRepo(project string) string
func (s Spec) ServiceRepo(project, service string) string
func (s Spec) TaskRepo(project, task string) string
func (s Spec) BackendSSMPrefix(env, project string) string
func (s Spec) ServiceSSMPrefix(env, project, service string) string
func (s Spec) TaskSSMPrefix(env, project, task string) string

// Independent second derivation. NOT used to resolve events.
func (s Spec) ParseRepo(project, repo string) (id string, ok bool)
func (s Spec) ExpectedRepo(project, id string) (repo string, ok bool)
```

`ParseRepo`, in order: exact match against `BackendRepo` → `backend`; anchored
`^{QuoteMeta(project)}_service_(.+)$` → `$1`; anchored `^{QuoteMeta(project)}_task_(.+)$` →
`task:$1`; otherwise `ok == false`. Anchoring plus `QuoteMeta` is the whole of the D2 fix on
the Go side; the repo allow-list in the event pattern is the other half.

### 5.2 `internal/config`

```go
type Kind string
const (KindService Kind = "service"; KindScheduledTask Kind = "scheduled_task")

type Target struct {
    ServiceName string `json:"service_name"`   // empty for scheduled tasks
    TaskFamily  string `json:"task_family"`
    Kind        Kind   `json:"type"`
}
type S3File struct { Bucket, Key string }

type Flags struct { ECR, SSM, S3, Manual bool }

type Config struct {
    Project, Env, Region string
    Cluster              string
    LogLevel             slog.Level
    Targets              map[string]Target      // ECS_SERVICE_MAP ∪ SCHEDULED_TASK_MAP
    ECRRepos             map[string][]string    // ECR_REPO_MAP     repo   -> identifiers
    SSMPrefixes          map[string]string      // SSM_SERVICE_MAP  prefix -> identifier
    S3Files              map[string][]S3File    // S3_SERVICE_MAP   id     -> files
    SlackWebhookURL      string
    MaxRetries           int
    RetryBaseDelay       time.Duration
    DryRun               bool
    Enable               Flags
}

func Load(getenv func(string) string) (*Config, error)
func (c *Config) Validate() error
func (c *Config) SelfCheck(spec contract.Spec) error

func (c *Config) Target(id string) (Target, bool)
func (c *Config) IsScheduledTask(id string) bool
func (c *Config) IdentifiersForRepo(repo string) []string
func (c *Config) IdentifierForSSMPath(path string) (string, bool)   // longest-prefix
func (c *Config) IdentifiersForS3(bucket, key string) []string
```

`IdentifierForSSMPath` normalises a leading `/`, then picks the longest key in `SSMPrefixes`
that is a path-segment prefix of the parameter name. No regex, no `\w+`, hyphens and dots are
non-events (D17), and `/dev/p/task/cleanup/env` resolves (D16).

### 5.3 `internal/awsecs`

```go
type API interface {
    UpdateService(context.Context, *ecs.UpdateServiceInput, ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
    DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
    RegisterTaskDefinition(context.Context, *ecs.RegisterTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
}

type Client struct { /* api, cluster, dryRun, log */ }
func New(api API, cluster string, dryRun bool, log *slog.Logger) *Client
func NewFromAWSConfig(cfg aws.Config, cluster string, dryRun bool, log *slog.Logger) *Client

type UpdateRequest struct {
    ServiceName    string
    TaskDefinition string   // family (ECS resolves latest ACTIVE), family:rev, or ARN
    Force          bool
}
type UpdateResult struct { ServiceName, TaskDefinition, DeploymentID string }

func (c *Client) UpdateService(ctx context.Context, req UpdateRequest) (UpdateResult, error)
func (c *Client) RegisterRevisionWithImage(ctx context.Context, family, imageURI string) (arn string, err error)
```

- `UpdateService` passes the **family** and reads the resolved ARN and PRIMARY deployment id
  out of the response. No list, no sort, no pagination — D3 disappears rather than being
  patched, and `ecs:ListTaskDefinitions` leaves the IAM policy.
- `RegisterRevisionWithImage` describes the family (which returns the latest ACTIVE revision
  by exact family name — also immune to D3's `FamilyPrefix` bug), clones **all** fields
  including `PidMode`, `IpcMode`, `InferenceAccelerators`, `Volumes`, `PlacementConstraints`,
  `RequiresCompatibilities`, `Cpu`, `Memory`, `ProxyConfiguration`, `EphemeralStorage`,
  `RuntimePlatform`, `TaskRoleArn`, `ExecutionRoleArn`, `NetworkMode`, and tags (D6), swaps
  images whose repo matches, and errors if zero containers matched (FR-12).
- `internal/awsecs/image.go`: `splitImageRef(ref) (repo, ref string)` — split on `@` first for
  digests, then on the last `:` only when it follows the last `/`. The current
  `strings.LastIndex(":")` mangles both digests and `host:port/repo`.

### 5.4 `internal/slack`

```go
type Level string   // "success" | "error" | "info"
type Message struct { Level Level; Env, Service, State, Reason, DeploymentID, TaskDef string }

type Notifier interface { Notify(ctx context.Context, m Message) }   // no error, by design

type Client struct { /* url, http, env, tmpl, log */ }
func New(webhookURL, env string, log *slog.Logger) Notifier   // returns noop{} when url == ""
func (c *Client) Notify(ctx context.Context, m Message)       // 5s sub-timeout, logs failures
```

There is no way to write `_ = slack.Notify(...)` and there is no way for `handleECSEvent` to
fail an invocation over a webhook (NFR-4). Templates move to
`internal/slack/templates/*.json.tmpl`, embedded, with a test that each renders **valid JSON**
for every `Message` shape — which the current dead templates would fail (D10).

### 5.5 `internal/deploy`

```go
type Source string   // "ecr" | "ssm" | "s3" | "manual"

type Request struct {
    ID             string   // "backend" | "{service}" | "task:{name}"
    ImageURI       string   // scheduled tasks only
    TaskDefinition string   // manual pin only
    Reason         string
    Source         Source
}
type Result struct { ID, ServiceName, TaskDefinition, DeploymentID string; Kind config.Kind }

type ECS interface {
    UpdateService(context.Context, awsecs.UpdateRequest) (awsecs.UpdateResult, error)
    RegisterRevisionWithImage(ctx context.Context, family, imageURI string) (string, error)
}

type Deployer struct { /* cfg, ecs, slack, log, sleep func(context.Context, time.Duration) error */ }
func New(cfg *config.Config, ecs ECS, n slack.Notifier, log *slog.Logger) *Deployer
func (d *Deployer) Deploy(ctx context.Context, req Request) (Result, error)
func (d *Deployer) DeployAll(ctx context.Context, reqs []Request) ([]Result, error)  // errors.Join
```

Order inside `Deploy`:

1. `cfg.Target(req.ID)`; miss → `ErrUnknownTarget` (wrapped, **outside** the retry loop, D13).
2. Notify `info` / `DEPLOYMENT_INITIATING`.
3. Retry loop over the AWS call only:
   `delay = base * 2^attempt` with ±20% jitter, `base = RetryBaseDelay` (default 1s),
   `sleep` aborts on `ctx.Done()`, and the loop stops early if the remaining deadline cannot
   fit another attempt. `retryable(err)` via `errors.As` on SDK v2 typed errors:
   retry `ThrottlingException`, `ServerException`, `RequestLimitExceeded`, timeouts;
   never retry `ServiceNotFoundException`, `ClusterNotFoundException`, `InvalidParameterException`,
   `ClientException`, `AccessDeniedException`.
4. Notify `success` / `error`. Return `(Result, nil)` or `(Result{}, err)`.

`DeployAll` is sequential (S3 fan-out is 1–3 services; concurrency buys nothing and risks
throttling), collects with `errors.Join`, and never aborts early.

### 5.6 `internal/handler`

```go
type Deployer interface {
    Deploy(context.Context, deploy.Request) (deploy.Result, error)
    DeployAll(context.Context, []deploy.Request) ([]deploy.Result, error)
}

type Response struct {
    Status   string   `json:"status"`             // "deployed" | "ignored" | "notified"
    Detail   string   `json:"detail,omitempty"`
    Deployed []string `json:"deployed,omitempty"`
}

type Handler struct { /* cfg, dep, slack, log */ }
func New(cfg *config.Config, dep Deployer, n slack.Notifier, log *slog.Logger) *Handler
func (h *Handler) Handle(ctx context.Context, ev events.CloudWatchEvent) (Response, error)

func (h *Handler) ecr(ctx context.Context, ev events.CloudWatchEvent) (Response, error)
func (h *Handler) ssm(ctx context.Context, ev events.CloudWatchEvent) (Response, error)
func (h *Handler) s3(ctx context.Context, ev events.CloudWatchEvent) (Response, error)
func (h *Handler) manual(ctx context.Context, ev events.CloudWatchEvent) (Response, error)
func (h *Handler) ecsState(ctx context.Context, ev events.CloudWatchEvent) (Response, error)
```

Error policy (FR-9, D14): return a non-nil error **only** when a retry could plausibly succeed
— i.e. a retryable AWS failure escaped `deploy`. Unknown source, unmapped repo, unparsable
detail, disabled feature flag, non-`PUSH` action → `Response{Status:"ignored"}, nil`.

Return type changes from `string` to `Response`. Safe: EventBridge invokes asynchronously and
discards the payload; the value is only visible in manual `aws lambda invoke` output.

`manual` additionally verifies `detail.project == cfg.Project && detail.env == cfg.Env` when
those fields are present, as defence in depth behind the rule filter (Q1).

### 5.7 `main.go`

```go
func main() {
    ctx := context.Background()
    log := newLogger(os.Getenv("LOG_LEVEL"))          // slog JSON, ReplaceAttr → timestamp/message/lowercase level
    cfg, err := config.Load(os.Getenv);               fatal(log, err)
    spec := contract.Load()
    fatal(log, cfg.SelfCheck(spec))                   // L3
    awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region)); fatal(log, err)
    log = log.With("project", cfg.Project, "env", cfg.Env)
    ecsClient := awsecs.NewFromAWSConfig(awsCfg, cfg.Cluster, cfg.DryRun, log)
    notifier  := slack.New(cfg.SlackWebhookURL, cfg.Env, log)
    h := handler.New(cfg, deploy.New(cfg, ecsClient, notifier, log), notifier, log)
    lambda.Start(h.Handle)
}
```

---

## 6. Environment variable contract

| Var | Shape | Change |
|---|---|---|
| `PROJECT_NAME`, `PROJECT_ENV`, `ECS_CLUSTER_NAME`, `LOG_LEVEL`, `SLACK_WEBHOOK_URL` | unchanged | — |
| `ECS_SERVICE_MAP` | `{"backend":{"service_name":…,"task_family":…}, "{svc}":{…}}` | unchanged shape; keys now come from `module.ci_identifiers` |
| `SCHEDULED_TASK_MAP` | `{"task:{n}":{"task_family":…,"type":"scheduled_task"}}` | unchanged shape; keys from the module |
| `ECR_REPO_MAP` | `{"acme_backend":["backend"],"acme_service_api":["api","reporting"]}` | **new** — replaces ECR repo-name parsing (D1, D2, D9) |
| `SSM_SERVICE_MAP` | `{"/dev/acme/backend":"backend","/dev/acme/task/cleanup":"task:cleanup"}` | **new** — replaces the SSM regex (D16, D17) |
| `S3_SERVICE_MAP` | `{"backend":[{"bucket":…,"key":…}], "{svc}":[…]}` | now includes backend files; bucket string is raw (D7, D8, Q2) |
| `MAX_DEPLOYMENT_RETRIES`, `DRY_RUN`, `ENABLE_*` | unchanged | — |
| `RETRY_BASE_DELAY_MS` | `"1000"` | **new**, optional |
| `SERVICE_CONFIG` | — | **removed** (never read) |
| `DEPLOYMENT_TIMEOUT_SECONDS` | — | **removed** (validated, never used) |

---

## 7. Terraform edits

### 7.1 `modules/workloads/lambda.tf`

**a. Identifiers.** Add the module call and replace the identifier-producing locals:

```hcl
module "ci_identifiers" {
  source               = "./ci_identifiers"
  project              = var.project
  env                  = var.env
  service_names        = keys(local.service_names)
  service_repo_names   = { for n, url in local.service_ecr_urls : n => element(split("/", url), length(split("/", url)) - 1) if url != "" }
  scheduled_task_names = var.scheduled_task_names
}
```

`ecs_service_map` / `scheduled_task_map` keep their current shape but take keys from
`module.ci_identifiers.backend_id`, `.service_ids[k]`, `.task_ids[n]`. Delete
`local.service_config` (FR-16). Rebuild `s3_to_service_map` from backend **and** service files
with raw bucket names (Q2).

**b. Build at apply (decision 4).**

```hcl
locals { ci_lambda_dir = "${path.module}/ci_lambda" }

resource "null_resource" "build_ci_lambda" {
  triggers = {
    src = sha1(join("", [
      for f in sort(tolist(fileset(local.ci_lambda_dir, "**/*.{go,json,mod,sum,tmpl}"))) :
      filesha1("${local.ci_lambda_dir}/${f}")
    ]))
  }
  provisioner "local-exec" {
    working_dir = local.ci_lambda_dir
    command     = <<-EOT
      command -v go >/dev/null || { echo "ERROR: Go toolchain required to build the CI Lambda (https://go.dev/dl). Install Go 1.22+ and re-run terraform apply."; exit 1; }
      GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bootstrap .
    EOT
  }
}

data "archive_file" "lambda" {
  type        = "zip"
  source_file = "${local.ci_lambda_dir}/bootstrap"
  output_path = "${local.ci_lambda_dir}/ci_lambda.zip"
  depends_on  = [null_resource.build_ci_lambda]
}
```

`aws_lambda_function.lambda_deploy`: `filename = data.archive_file.lambda.output_path`, add
`architectures = ["arm64"]` (Q3). Keep `source_file` (not `source_dir`) — the executable-bit
behaviour that works today is preserved.

**c. Four event rules** (FR-14; this is the part decision 3 cannot do in one rule):

```hcl
resource "aws_cloudwatch_event_rule" "ecr_push" {
  name = "${var.project}_ci_ecr_${var.env}"
  event_pattern = jsonencode({
    source      = ["aws.ecr"]
    detail-type = ["ECR Image Action"]
    detail = {
      action-type     = ["PUSH"]
      result          = ["SUCCESS"]
      repository-name = keys(module.ci_identifiers.ecr_repo_ids)   # same expression as ECR_REPO_MAP (FR-15)
    }
  })
}

resource "aws_cloudwatch_event_rule" "ecs_state" {
  name = "${var.project}_ci_ecs_${var.env}"
  event_pattern = jsonencode({
    source      = ["aws.ecs"]
    detail-type = ["ECS Deployment State Change", "ECS Service Action"]
    resources   = [{ prefix = "${replace(aws_ecs_cluster.main.arn, ":cluster/", ":service/")}/" }]
  })
}

resource "aws_cloudwatch_event_rule" "ssm_change" {
  name = "${var.project}_ci_ssm_${var.env}"
  event_pattern = jsonencode({
    source      = ["aws.ssm"]
    detail-type = ["Parameter Store Change"]
    detail      = { name = [{ prefix = "/${var.env}/${var.project}/" }] }
  })
}

resource "aws_cloudwatch_event_rule" "manual_deploy" {
  name = "${var.project}_ci_manual_${var.env}"
  event_pattern = jsonencode({
    source      = ["action.deploy", "action.production", "action.${var.env}"]
    detail-type = ["DEPLOY"]
    detail      = { project = [var.project], env = [var.env] }      # Q1
  })
}
```

The ECS rule's `resources` prefix is the fix for cross-project Slack noise — the ECS half of
D2, which the stated decision 3 does not cover. Cluster ARN
`arn:…:cluster/acme_cluster_dev` becomes service-ARN prefix
`arn:…:service/acme_cluster_dev/`.

Each rule gets its own `aws_cloudwatch_event_target` and `aws_lambda_permission` with a unique
`statement_id` (`AllowEventBridgeECR`, `…ECS`, `…SSM`, `…Manual`) — the current hardcoded
`AllowExecutionFromCloudWatch` collides across four rules. Optionally add `retry_policy` +
`dead_letter_config` to the targets (Q4). The old `aws_cloudwatch_event_rule.ecr_event`,
target, and permission are destroyed; expect a one-time replace in the plan.

**d. Env vars.** Add `ECR_REPO_MAP = jsonencode(module.ci_identifiers.ecr_repo_ids)` and
`SSM_SERVICE_MAP = jsonencode(module.ci_identifiers.ssm_prefix_ids)`; remove `SERVICE_CONFIG`
and `DEPLOYMENT_TIMEOUT_SECONDS`.

**e. IAM** (FR-18): drop `ecs:ListTaskDefinitions`; scope `ecs:UpdateService` to
`concat([aws_ecs_service.backend.id], values(aws_ecs_service.services)[*].id)`; keep
`DescribeTaskDefinition`/`RegisterTaskDefinition`/`TagResource` on `*` (no resource-level
support); keep `iam:PassRole` on `*` but add
`condition { test = "StringEquals", variable = "iam:PassedToService", values = ["ecs-tasks.amazonaws.com"] }`.
Scheduled-task roles live in `modules/ecs_task`, so a resource-scoped PassRole would need new
cross-module plumbing — the condition is the meaningful restriction without it.

### 7.2 Other files

| File | Edit |
|---|---|
| `modules/workloads/variables.tf` | `lambda_path` kept, marked deprecated/unused (removing it breaks already-generated `env/*/main.tf`). |
| `env/main.hbs:507` | Drop the `lambda_path` line so new generations omit it. |
| `app/terrafrom.go:39-78,82` and `app/terraform_destroy_progress_tui.go:153` | **Delete `ensureLambdaBootstrapExists` and both call sites** (D12). With the null_resource build, its only remaining effect is to smuggle a dummy artifact into the zip. |
| `app/deploy.go:76,465-484` | Delete `buildDeploymentLambda` and the `env/<env>/ci_lambda.zip` removal — one build path only (Terraform). |
| `.gitignore` | Add `modules/workloads/ci_lambda/ci_lambda.zip`. `bootstrap` is already at line 32. |
| `Taskfile.yml:118-137`, `project/Taskfile.yml:74-79` | `lambda:build` → `go build … -o bootstrap .`; add `lambda:test`, `lambda:test:boundary` (`-tags tfboundary`), `lambda:lint`. |
| `.github/workflows/ci.yml` | **New** (D11): on PRs touching `modules/workloads/**`, run `go vet`, `go test ./...`, `go test -tags tfboundary ./contract/...`, `terraform fmt -check`, `terraform validate`. |
| `web/src/components/Sidebar.tsx:769,878`, `web/src/components/ServiceCICDConfiguration.tsx:123-127`, `docs/ECR_STRATEGY.md:316-339` | Only if Q1 is answered "scope the events": emit `Source=action.deploy`, `DetailType=DEPLOY`, `Detail={"project":…,"env":…,"service":…}`, then rebuild `app/webapp/` (pre-commit hook does this). |

---

## 8. Testing strategy

| Layer | What | Where |
|---|---|---|
| Contract unit | `Format*`/`Parse*` round-trip for backend/service/task; hyphens, dots; `otherproj_*` rejected under project `acme`; `acme_task_cleanup_extra` ≠ `acme_task_cleanup` | `contract/contract_test.go` |
| Static guard | `lambda.tf` contains no identifier literals and does reference `module.ci_identifiers` | `contract/lambdatf_guard_test.go` |
| **Boundary** | **Executes Terraform**, feeds real outputs to real `config.Load` + `SelfCheck` + resolvers; fan-out and cross-project cases | `contract/tfboundary_test.go` (tag `tfboundary`) |
| Config | `Load` via injected `getenv`; malformed JSON; missing required; `SelfCheck` catches a deliberately corrupted map | `internal/config/config_test.go` |
| ECS | Fake `API`: `UpdateService` receives the family and not a computed ARN; `RegisterRevisionWithImage` carries `PidMode`/`IpcMode`/`InferenceAccelerators`; zero-container-match errors; digest and `host:port` image refs | `internal/awsecs/*_test.go` |
| Deploy | Real `Deployer` (not a copy — D15): retry counts, backoff growth via injected `sleep`, non-retryable fails once, Slack failure does not affect the result, unknown identifier never reaches AWS | `internal/deploy/deployer_test.go` |
| Handler | Table of golden events per source; `ignored` vs error classification; ECS-state never errors; ECR fan-out deploys N | `internal/handler/*_test.go` |
| Slack | Every template × every `Message` renders valid JSON (`json.Valid`) | `internal/slack/client_test.go` |
| Manual smoke | Documented `aws lambda invoke` payloads for backend push, service push, task push, SSM write, S3 write, manual deploy | `README.md` |

Shared fixture (`internal/testsupport`) builds the synthetic project once and hands every test
the same env-var set, so no test can invent map keys the way `handlers/handler_test.go:81` and
`config_test.go:27` do today.

---

## 9. Build sequence (tree compiles and tests pass at every step)

| # | Step | Compiles? |
|---|---|---|
| 0 | Add `.github/workflows/ci.yml` running the existing tests. Establishes the baseline and proves D11. | yes |
| 1 | Add `contract/` (json, `contract.go`, tests). New leaf package, nothing imports it yet. | yes |
| 2 | Add `modules/workloads/ci_identifiers/`; `terraform validate`. No Go impact. | yes |
| 3 | Add `contract/tfboundary_test.go` + static guard. Guard fails against current `lambda.tf` → keep it skipped-with-TODO or land it in step 8. | yes |
| 4 | `go.mod`: `go 1.22`, add `aws-sdk-go-v2/{config,service/ecs}`, bump `aws-lambda-go`/`testify`. Keep v1 for now. | yes |
| 5 | Add `internal/config` + `internal/testsupport`. New import paths, old `config/` untouched. | yes |
| 6 | Add `internal/slack`, `internal/awsecs` (SDK v2) with fakes and tests. | yes |
| 7 | Add `internal/deploy`, `internal/handler`. | yes |
| 8 | **Switch-over commit** (unavoidably atomic — one `package main` per dir): rewrite `main.go`; delete `config/`, `services/`, `deployer/`, `handlers/`, `utils/`, `cmd/`, `*.tmpl`, `Makefile`. `go mod tidy` drops `aws-sdk-go` v1. | yes, after |
| 9 | Terraform: identifiers module wiring, 4 rules, env vars, null_resource build, IAM. Enable the static guard. | n/a |
| 10 | CLI/tooling: delete `ensureLambdaBootstrapExists` + `buildDeploymentLambda`, update Taskfiles, `.gitignore`, `env/main.hbs`, README. | yes |
| 11 | Optional per Q1: web generators + docs + `app/webapp` rebuild. | yes |
| 12 | Deploy to a scratch project; run the manual smoke matrix; verify a second project in the same account is unaffected. | — |

Steps 1–7 are individually reviewable and independently revertable. Step 8 is the only
irreversible-in-one-commit point.

---

## 10. Risks

| Risk | Mitigation |
|---|---|
| Go absent on an apply machine | Provisioner pre-flights `command -v go` with an actionable message; documented in README and release notes. |
| First plan after the change shows churn (`archive_file` deferred by `depends_on`) | Expected, matches `modules/appsync/auth_lambda.tf`; call it out in the changelog. |
| Old EventBridge rule replaced by four new ones | Brief window during apply where no rule is attached; deployments triggered in that window are lost. Apply during a quiet period, or accept — the current single rule is already mis-scoped. |
| Existing Logs Insights queries on `fields.*` break | Documented in NFR-3; attributes become top-level and easier to query. |
| `arm64` switch (Q3) | Reversible in one attribute; unrelated to correctness. |
| Q1 not answered | Manual deploys stay cross-project-unsafe and generated non-prod workflows stay dead. This is a pre-existing bug, not a regression, but the rewrite is the moment to fix it. |
| `use_existing` fan-out changes behaviour | A push to a shared repo now deploys every consumer instead of just the owner. That is the intended semantics of `use_existing`; note it in the changelog. |
