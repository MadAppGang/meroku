# GitHub Actions Runners on CodeBuild

`github_runners` gives an environment its own GitHub Actions runners, hosted by
AWS CodeBuild. A workflow job that asks for one runs in this AWS account, as a
fresh container that exists for that one job and is gone afterwards. Nothing
stays running between jobs, and nothing is paid for between jobs.

## How it works

For each runner in the YAML, meroku creates:

- a **CodeBuild project** named `<project>-gh-<runner>-<env>`, with a GitHub
  source and an empty buildspec;
- a **webhook** on that project that fires on `WORKFLOW_JOB_QUEUED`. CodeBuild
  creates the matching webhook in GitHub itself, using the token;
- a **CloudWatch log group** `<project>_gh_runner_<runner>_<env>`, kept 7 days.

Shared by all runners in the environment:

- one **IAM service role** (`<project>_gh_runners_<env>`, through
  `modules/naming`) with one inline policy;
- when `vpc_access: true`, one **security group** `<project>_gh_runners_<env>`
  with all egress and no ingress.

When GitHub queues a job whose `runs-on` names the project, the webhook starts a
build. The build registers itself with GitHub as an ephemeral self-hosted
runner, runs the job's steps, and exits.

The GitHub credential is set on each project (`source.auth`, type
`SECRETS_MANAGER`). meroku never creates an `aws_codebuild_source_credential`:
that resource is one per account and server type, so two environments in the
same account would overwrite each other's token.

Code: `modules/github_runners/`, wired in `env/main.hbs`; validation in
`app/github_runners.go`.

## YAML

```yaml
github_runners:
  enabled: true
  token_secret: meroku/github-runners/myapp-dev   # Secrets Manager secret NAME (or ARN)
  vpc_access: false                               # default false
  runners:
    - name: default                               # [a-z0-9-], unique
      repository: example-org/some-repo           # owner/repo
      # organization: example-org                 # instead of repository: every repo in the org
      compute_type: BUILD_GENERAL1_SMALL          # SMALL (default) | MEDIUM | LARGE | XLARGE | 2XLARGE
      image: aws/codebuild/amazonlinux-x86_64-standard:5.0   # default
      docker: true                                # privileged mode, default false
      timeout_minutes: 60                         # 5-2160, default 60
      env_variables:
        - name: FOO
          value: bar
      secrets:
        - name: DB_PASSWORD
          ssm: /myapp/dev/db_password             # Parameter Store name
        - name: API_KEY
          secrets_manager: myapp/dev/api:key      # name-or-arn[:json-key]
```

- Absent, or `enabled: false`, renders nothing.
- An image whose name contains `aarch64` runs on `ARM_CONTAINER`; any other on
  `LINUX_CONTAINER`.
- `env_variables` become `PLAINTEXT` variables, `ssm` entries `PARAMETER_STORE`
  and `secrets_manager` entries `SECRETS_MANAGER`. CodeBuild resolves the last
  two when the build starts; their values never reach Terraform state.
- No schema migration: the block is optional and absent on every existing
  environment, and absence is the correct state.

The validation rules are in
[VALIDATION_RULES_CATALOG.md §15](./VALIDATION_RULES_CATALOG.md#15-github-actions-runners-github_runners).

## Using a runner in a workflow

`runs-on` must name the CodeBuild project and carry the run ID and attempt, so
each job maps to exactly one build:

```yaml
jobs:
  build:
    runs-on: codebuild-myapp-gh-default-dev-${{ github.run_id }}-${{ github.run_attempt }}
```

The environment outputs the exact value per runner, so it does not have to be
assembled by hand:

```bash
terraform output github_runner_labels
# {
#   "default" = "codebuild-myapp-gh-default-dev-${{ github.run_id }}-${{ github.run_attempt }}"
# }
```

`github_runner_projects` maps each runner to its CodeBuild project name.

The runner's role cannot deploy. To push images or update ECS services, the job
assumes the environment's GitHub OIDC role (`workload.enable_github_oidc`) with
`aws-actions/configure-aws-credentials`, exactly as on a GitHub-hosted runner.

## The token secret

CodeBuild reads the GitHub token from a Secrets Manager secret that you create
before the first apply; Terraform only looks it up by name, so the token never
enters the state file. The apply fails if the secret does not exist.

The secret value is JSON:

```json
{"ServerType":"GITHUB","AuthType":"PERSONAL_ACCESS_TOKEN","Token":"ghp_fake-token-do-not-use"}
```

and it carries two tags CodeBuild requires:

| Tag | Value |
|-----|-------|
| `codebuild:source:provider` | `github` |
| `codebuild:source:type` | `personal_access_token` |

```bash
aws secretsmanager create-secret \
  --name meroku/github-runners/myapp-dev \
  --secret-string '{"ServerType":"GITHUB","AuthType":"PERSONAL_ACCESS_TOKEN","Token":"ghp_fake-token-do-not-use"}' \
  --tags Key=codebuild:source:provider,Value=github Key=codebuild:source:type,Value=personal_access_token
```

Token scopes (classic personal access token):

- `repo`, for a repository runner. The token's user must be an admin of the
  repository, because CodeBuild creates the repository webhook with it.
- `repo` and `admin:org_hook`, for an organization runner, which needs an
  organization webhook.

One secret per environment is the simplest arrangement; nothing stops two
environments sharing one.

## `vpc_access` and the NAT rule

By default builds run outside the VPC, on CodeBuild's own network, with direct
internet access. That is enough for anything that reaches AWS through its public
endpoints.

`vpc_access: true` places the builds in the environment's **private** subnets,
for jobs that must reach something only the VPC can: an RDS instance without
public access, an internal service, a private DNS name.

CodeBuild never gives a build in a VPC a public IP address. Its only way to the
internet, and so to GitHub, is a NAT Gateway. Without one the build cannot
register as a runner and every job stays queued until it times out. So meroku
refuses, at every generate:

- `vpc_access: true` with `egress_strategy` `public_ip` (or absent, which means
  `public_ip`). Fix: set `egress_strategy: nat_gateway` (or `nat_gateway_ha`),
  or set `vpc_access: false`.
- `vpc_access: true` with `use_default_vpc: true`. The default VPC has no
  private subnets and no NAT.

Because the check runs on every generate, switching `egress_strategy` back to
`public_ip` later is refused for the same reason.

With `vpc_access`, the module receives `module.vpc.private_subnet_ids` and the
role also gets the EC2 network-interface permissions AWS documents for VPC
builds; `ec2:CreateNetworkInterfacePermission` is limited to those subnets and
to `codebuild.amazonaws.com`.

## IAM

One inline policy on the service role, each statement scoped to what this
module created or the YAML named:

| Statement | Actions | Resources |
|-----------|---------|-----------|
| `WriteRunnerLogs` | `logs:CreateLogStream`, `logs:PutLogEvents` | the runners' log groups |
| `ReadSecrets` | `secretsmanager:GetSecretValue` | the token secret, and each `secrets_manager` secret (a name gets the `-??????` suffix wildcard Secrets Manager ARNs carry) |
| `ReadParameters` | `ssm:GetParameters` | each `ssm` parameter (only when one is listed) |
| `ManageBuildNetworkInterfaces`, `AttachBuildNetworkInterfaces` | EC2 network-interface actions | only with `vpc_access` |

The trust policy admits `codebuild.amazonaws.com` only with
`aws:SourceAccount` equal to this account.

A `SecureString` parameter or a secret encrypted with a customer-managed KMS
key also needs `kms:Decrypt` on that key, which this role does not have. The
AWS-managed keys (`aws/ssm`, `aws/secretsmanager`) need nothing extra.

## Cost

- **Build minutes.** CodeBuild bills per minute of build time, by compute type
  and OS; an idle runner costs nothing. See the CodeBuild pricing page for the
  current rates.
- **NAT data.** With `vpc_access`, everything a job downloads (base images,
  packages, caches, artifacts) passes through the NAT Gateway at **$0.045/GB**
  processed, on top of the NAT's hourly charge. CI is download-heavy, so leave
  `vpc_access` off unless a job really needs the VPC. See
  [EGRESS_COST_MODEL.md](./EGRESS_COST_MODEL.md).
- **Logs.** CloudWatch Logs ingestion; retention is 7 days.

## Not supported yet

- **Reserved capacity fleets** (`aws_codebuild_fleet`). Every runner is
  on-demand.
- **A web UI panel.** The block is edited in the YAML; the web UI keeps it
  intact on save but does not show it.
- **GPU compute** and Lambda compute types.
- GitHub Enterprise Server, GitHub App or CodeConnections credentials. Only a
  personal access token in Secrets Manager.
- Per-job overrides in the `runs-on` label (`image:`, `instance-size:`). The
  image and size are the project's.

## Tests

```bash
cd app && go test -run GitHubRunners ./...          # validation, rendering, YAML round-trip
terraform -chdir=modules/github_runners init -backend=false
terraform -chdir=modules/github_runners test        # mock_provider, plan only
```
