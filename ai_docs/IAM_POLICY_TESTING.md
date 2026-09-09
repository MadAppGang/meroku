# Testing an IAM policy document in this repo

Why a policy document inside `modules/workloads` cannot be tested where it sits,
what to do instead, and how to check the result against real AWS without
deploying anything.

Written while scoping the GitHub Actions role's ECR grants in v4.8.0. The
constraint is not specific to that policy — it applies to every
`aws_iam_policy_document` in `modules/workloads`.

## The constraint

Every test file in `modules/workloads/tests/` must declare:

```hcl
mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}" }
  }
}
```

It is not optional. A generated mock string is not a policy document, and the
AWS provider validates that **client-side**, so without the stub seven resources
in the module fail with `"policy" contains an invalid JSON policy` before a
single assertion runs.

The consequence is easy to miss: **`data.aws_iam_policy_document.<name>.json` is
structurally unreadable from a test in that directory.** Whatever the statement
blocks say, the attribute returns the stub.

`override_data` does not rescue it. It *replaces* a mocked address's values with
hand-written ones — it moves in the same direction as the mock. There is no
construct that un-mocks one address back to the real provider.

The root cause is that `modules/workloads` reads eight remote data sources, so it
can never be planned without AWS credentials. `.github/workflows/ci.yml` already
records the same discovery for precondition messages, which "live in a module
with no provider at all" for exactly this reason.

## The trap this creates

Asserting on the `locals` that feed the statement looks like a reasonable
substitute. It is not sufficient, and the gap is silent:

```hcl
statement {
  actions   = local.scoped_actions
  resources = ["*"]          # locals still correct, and now unread
}
```

Both locals stay perfect, every assertion passes, and the grant is back to a
wildcard. This was reproduced: the full suite reported `3 passed, 0 failed` with
the security property entirely reverted.

## The fix: extract a leaf module

Move the policy document into its own module that takes account and region as
**inputs** rather than reading them:

```hcl
variable "aws_account_id" {}   # not data.aws_caller_identity
variable "region" {}           # not data.aws_region
output "json" { value = data.aws_iam_policy_document.<name>.json }
```

With no data sources, the module plans with no credentials and no network —
`aws_iam_policy_document` renders client-side — so `terraform test` can assert on
the **real rendered JSON**. `modules/github_policy/` is the worked example;
`modules/naming/` and `modules/env_secret_check/` are the same shape for
different reasons.

Wire the CI step next to the `modules/naming` one in `.github/workflows/ci.yml`.

Extraction relocates some properties out of the unit. A module that receives
`region` as a string cannot tell where the string came from, so "the caller
passes *this* region" becomes the caller's property and needs its own guard.
Check both sides after extracting.

## Assert by matching, not by string shape

A string-shape assertion (`strcontains(arn, ":repository/acme")`) passes for a
prefix that is one character too loose. Turn each granted `Resource` into the
glob IAM treats it as — escape regex metacharacters, then expand only `*` and
`?`, then anchor both ends — and ask whether a candidate ARN is covered.

`.tftest.hcl` files have no `locals`, so the matcher lives in a small module the
`run` block points at (`modules/github_policy/tests/matcher/`).

Then test the negative cases, which are the requirement:

| Candidate | Must be |
|---|---|
| `<project>_backend`, `<project>_service_*`, `<project>_task_*` | reachable |
| another project's repositories | unreachable |
| `<project>corp_backend` — a longer project name | unreachable |
| an unprefixed repository, e.g. plain `backend` | unreachable |

That last row is real: an account in production held a repository named `backend`
matching no project convention. Only listing the account's actual repositories
surfaced it.

This matcher models a shallow IAM — no `Condition` keys, no
`NotAction`/`NotResource`, no explicit-`Deny` precedence, no resource-policy
side. If a statement starts using any of those, the matcher must grow first or
it will silently misstate.

## Checking against real AWS, read-only

`iam:SimulateCustomPolicy` evaluates a policy document with AWS's own engine. It
creates nothing and does not need the role to exist, so it is safe against a live
account.

1. Extract the locals and the policy document out of the `.tf` file **with
   `sed`**, not by retyping, into a scratch harness outside the repo. Retyping
   means you validate a copy, not the thing that ships.
2. Render it. Provider needs only `region`, plus
   `skip_credentials_validation`, `skip_requesting_account_id`,
   `skip_metadata_api_check` if you have no credentials.
3. Simulate the old policy and the new one through the same calls and diff the
   decisions:

```bash
aws iam simulate-custom-policy \
  --policy-input-list "$(jq -c . policy.json)" \
  --action-names ecr:PutImage \
  --resource-arns "arn:aws:ecr:<region>:<account>:repository/<other-project>_backend" \
  --query 'EvaluationResults[0].EvalDecision' --output text
```

`allowed` → `implicitDeny` on the cross-project rows, with every own-project row
still `allowed`, is the proof. `--policy-input-list` needs **compact** JSON;
pretty-printed input fails with `Policy input list item 1 has invalid content`.

Keep the output out of the repo. This repository is public and simulation output
carries real account IDs.

## Two ECR facts worth not rediscovering

- `ecr:GetAuthorizationToken` is evaluated at the registry and has no resource.
  Listing it alongside repository ARNs denies it, which breaks
  `aws-actions/amazon-ecr-login` and therefore every push. It needs its own
  statement on `"*"`.
- `ecr:CreateRepository` **is** resource-scopable, and generated workflows do call
  it (`describe-repositories || create-repository`). It cannot be authorised
  against an enumerated name list, because the repository being created is by
  definition not in the list. A prefix works; an enumeration does not.

## A Go text guard is the complement, not the substitute

Some properties are absences and cannot be planned: that no
`aws_iam_role_policy_attachment` targets the role, that no second inline policy
exists, that the caller passes the right inputs. Those belong in
`modules/workloads/ci_lambda/internal/boundary/`, using the existing
`workloadsTF(t, name)` helper, which strips comments first — comments in these
files quote the very constructs being guarded against.

Keep such guards to properties the plan cannot see. A text guard that greps a
file for strings copied out of that same file proves only that the file agrees
with itself, and will reject correct refactors.

One operational catch: those guards read `.tf` files three directories above the
Go module, so `go test` cannot hash them and returns `ok (cached)` after the
Terraform changes. `Taskfile.yml`'s `lambda:test` and `ci.yml` both pass
`-count=1`; keep it that way.
