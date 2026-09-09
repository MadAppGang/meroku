# ---------------------------------------------------------------------------
# A tiny IAM evaluator, for tests only. Nothing in modules/ consumes it.
#
# WHY IT EXISTS
#
# The requirement is not "the policy contains this substring". It is: given a
# repository that exists in this AWS account, can this role's policy reach it?
# That question is answered by MATCHING a repository's ARN against the Resource
# patterns the policy grants — which is what IAM itself does, and what
# iam:SimulateCustomPolicy did once by hand in ai-docs/sessions/
# dev-fix-20260909-ecr-scope/real-validation.md. A test that asserts on the
# SHAPE of the granted strings (`startswith("arn:aws:ecr:")`,
# `strcontains(":repository/acme")`) cannot answer it: both of those hold for
# `arn:aws:ecr:us-east-1:000000000000:repository/acme*`, which reaches a
# neighbouring project called `acmecorp`, and both hold for an ARN with an empty
# region field, which reaches nothing at all.
#
# It is a MODULE and not a `locals` block in the test file because .tftest.hcl
# files have no locals. A `run` block may point at a module of its own
# (`module { source = "./tests/matcher" }`), so the expression lives here once
# instead of being pasted into six assertions.
#
# WHAT IT DOES NOT MODEL
#
# Deliberately shallow, because the document under test is shallow: no
# Condition keys, no NotAction/NotResource, no resource-policy side, no explicit
# Deny precedence beyond reporting Effect. If a future statement needs any of
# those, this module must grow before it is trusted — an unmodelled Condition
# would be silently ignored and the reach reported here would be an
# overstatement. `effects` is exported precisely so a test can assert that the
# unmodelled constructs are still absent.
# ---------------------------------------------------------------------------

variable "policy_json" {
  description = "The rendered policy, exactly as the role receives it — module.github_policy.json."
  type        = string
}

variable "candidates" {
  description = <<-EOT
    Label -> the full ARN of a resource that may or may not exist. The labels
    are what failures name, so they should read like the repository they stand
    for ("acme_backend", "circl_backend").

    ARNs are given whole rather than assembled here: the account and region
    halves of the granted patterns are as much a part of the requirement as the
    repository name, and building the candidate from the same inputs as the
    grant would make those two halves match by construction.
  EOT
  type        = map(string)
}

variable "registry_level_actions" {
  description = <<-EOT
    ECR actions IAM evaluates at the REGISTRY rather than at a repository. They
    can only ever be granted on Resource "*", so they match every candidate ARN
    a glob comparison is given — including a neighbouring project's — while
    granting nothing against that repository. Subtracted in
    `ecr_repository_reach` so "the neighbour is unreachable" can be stated as
    the empty list it morally is.

    This is a carve-out, so it is deliberately a short, explicit list rather
    than a prefix rule: anything added here stops being checked against the
    cross-project property, and that is a decision worth making one action at a
    time.
  EOT
  type        = set(string)
  default     = ["ecr:GetAuthorizationToken"]
}

locals {
  # Statements, normalised. A rendered aws_iam_policy_document emits a bare
  # STRING for a single-element Action or Resource and a LIST for several, so
  # every consumer that forgets the difference works on one statement and
  # crashes on the next. flatten([x]) accepts both.
  statements = [
    for s in jsondecode(var.policy_json).Statement : {
      effect    = s.Effect
      actions   = flatten([s.Action])
      resources = flatten([s.Resource])

      # An IAM Resource is a glob, not a regex: `*` matches any run of
      # characters and `?` matches exactly one. Everything else is literal, so
      # every other regex metacharacter has to be escaped BEFORE the two
      # wildcards are expanded — otherwise a `.` in a repository name
      # (`acme_service_foo.bar` is a legal ECR name) would match any character
      # and the test would report reach the policy does not grant.
      #
      # Anchored at both ends, because IAM matches the WHOLE ARN. Unanchored,
      # `repository/acme_backend` would match
      # `repository/acmecorp_backend_shadow` and this module would agree with
      # every wrong implementation it exists to catch.
      patterns = [
        for r in flatten([s.Resource]) :
        format("^%s$", replace(replace(replace(r, "/[.+^$(){}\\[\\]|\\\\]/", "\\$0"), "*", ".*"), "?", "."))
      ]
    }
  ]

  # The answer, per candidate: every action any Allow statement grants on that
  # exact ARN. An empty list means IAM's implicit deny — the role cannot touch
  # that resource at all, which is the state every neighbouring project's
  # repository has to be in.
  reach = {
    for label, arn in var.candidates : label => sort(distinct(flatten([
      for s in local.statements : s.actions
      if s.effect == "Allow" && anytrue([for p in s.patterns : length(regexall(p, arn)) > 0])
    ])))
  }
}

output "reach" {
  description = <<-EOT
    Label -> every action allowed on that ARN, from all services. Not the
    assertion surface — the ecs:* and events:PutEvents grants in this policy are
    still on Resource "*" by agreed scope (real-validation.md, last row), so
    this is non-empty for every candidate including a neighbour's. It is
    exported for failure messages, which read far better naming what IS reachable
    than reporting that a filtered list was not empty.
  EOT
  value       = local.reach
}

output "ecr_repository_reach" {
  description = <<-EOT
    Label -> the ECR actions allowed AGAINST THAT REPOSITORY: the ECR half of
    `reach`, minus var.registry_level_actions. This is the assertion surface and
    the whole point of the module.

    For a repository of this project it must be the full push path. For every
    other project's repository in the same account it must be the empty list —
    IAM's implicit deny, and the exact result iam:SimulateCustomPolicy returned
    in real-validation.md.
  EOT
  value = {
    for label, actions in local.reach : label => [
      for a in actions : a if startswith(a, "ecr:") && !contains(var.registry_level_actions, a)
    ]
  }
}

output "effects" {
  description = <<-EOT
    Every distinct Effect in the document. A test asserts this is exactly
    ["Allow"]: an explicit Deny beats every Allow in IAM and is not recoverable
    by any other policy, so a statement that flips to Deny does not narrow this
    role, it stops every push in every project permanently — and this module
    does not model Deny precedence, so its `reach` would keep reporting the
    grant as if nothing had changed.
  EOT
  value       = sort(distinct([for s in local.statements : s.effect]))
}

output "actions" {
  description = "Every action the document grants anywhere, deduplicated. Used to assert the action set is EXACT rather than a superset."
  value       = sort(distinct(flatten([for s in local.statements : s.actions])))
}

output "malformed_ecr_resources" {
  description = <<-EOT
    Every granted ECR Resource that is a syntactically valid ARN matching
    NOTHING, because a field it needs is empty or missing. `arn:aws:ecr::
    000000000000:repository/acme_backend` — no region — is accepted by IAM, shows
    up in a plan diff as a real grant, and denies at the call; so does an ARN
    with an empty account field. This is the failure mode
    modules/github_policy/main.tf guards ecr_account_region against and that
    nothing tested before: it is invisible to any assertion phrased as
    `startswith(arn, "arn:aws:ecr:")`, which such an ARN passes.
  EOT
  value = [
    for r in distinct(flatten([for s in local.statements : s.resources])) : r
    if startswith(r, "arn:aws:ecr:") && (
      length(split(":", r)) != 6 ||
      try(split(":", r)[3], "") == "" ||
      try(split(":", r)[4], "") == "" ||
      try(split(":", r)[5], "") == ""
    )
  ]
}
