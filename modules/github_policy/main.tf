# ---------------------------------------------------------------------------
# The IAM policy the GitHub Actions deploy role carries, as a leaf module.
#
# WHY IT IS A MODULE OF ITS OWN
#
# This document used to sit in modules/workloads/github.tf, where no test could
# read what it rendered. Every test file in modules/workloads/tests/ has to
# declare `mock_data "aws_iam_policy_document"` with a stub json string — a
# generated mock value is not a policy document and the AWS provider validates
# that CLIENT-side, so without the stub seven resources in that module fail with
# `"policy" contains an invalid JSON policy` before a single assertion runs. The
# consequence was that `data.aws_iam_policy_document.github.json` was
# structurally unreadable from a test there: whatever the statement blocks said,
# the attribute returned the stub. `override_data` does not rescue it either —
# it REPLACES a mocked address's values with hand-written ones, moving in the
# same direction as the mock, and there is no construct that un-mocks an address
# back to the real provider.
#
# The document was not unreachable because policy documents are hard to test. It
# was unreachable because the unit under test was not a unit: it was welded into
# a 40-file module whose seven other policy resources force the mock that
# destroys the value. Extracting it removes the mock, and the extracted module
# renders the real JSON under the real provider — `aws_iam_policy_document` is
# computed client-side, and there is deliberately no data source here, so a test
# needs no credentials and no network.
#
# What that buys, concretely: a test can now ask the questions the requirement
# actually asks — does a neighbouring project's repository name match any
# granted Resource, is every statement's Effect still Allow, is any ECR write
# action granted on "*" — none of which can be expressed as an assertion about a
# local, and none of which the previous arrangement could see.
#
# The locals below are kept even though the statements could inline them. They
# are the named security surface and the shape the previous test suite asserted
# on; keeping the names stable keeps those assertions portable. They are also
# exported (outputs.tf) so a test can read the inputs to the JSON as well as the
# JSON itself.
#
# The repo already had this exact pattern before this module existed:
# modules/naming + modules/naming/tests/cascade.tftest.hcl, wired into CI at
# .github/workflows/ci.yml.
# ---------------------------------------------------------------------------

locals {
  # The ECR actions AWS supports resource-level permissions for. All eight used
  # to sit on "*" together with GetAuthorizationToken, which in an account
  # hosting two meroku projects let each project's CI role push an image over
  # the other's tags, or create repositories inside the other's namespace. The
  # push path is unchanged; only its reach is.
  #
  # ecr:GetAuthorizationToken is deliberately ABSENT. It is evaluated at the
  # registry, not at a repository — there is no resource for IAM to match it
  # against, so listing it alongside repository ARNs denies it outright. That
  # call is what aws-actions/amazon-ecr-login makes in every workflow meroku
  # generates (web/src/components/Sidebar.tsx,
  # web/src/components/ServiceCICDConfiguration.tsx), so scoping it does not
  # narrow a blast radius, it breaks `docker login` and therefore every push in
  # every project. It keeps its own statement on "*" below.
  github_ecr_scoped_actions = [
    "ecr:BatchCheckLayerAvailability",
    "ecr:BatchGetImage",
    "ecr:CompleteLayerUpload",
    "ecr:CreateRepository",
    "ecr:DescribeRepositories",
    "ecr:InitiateLayerUpload",
    "ecr:PutImage",
    "ecr:UploadLayerPart",
  ]

  # This project's ECR namespace, as PREFIXES rather than as an enumeration of
  # repository names.
  #
  # The three patterns are the three templates that actually create the
  # repositories: modules/workloads/ecr.tf:5 (backend),
  # modules/workloads/ecr.tf:234 (services) and modules/ecs_task/main.tf:62
  # (scheduled tasks). They are also what every generated workflow pushes to —
  # Sidebar.tsx:788 and :899, ServiceCICDConfiguration.tsx:86-91 construct
  # exactly these three names — and what CLAUDE.md's naming table documents.
  #
  # PREFIXES, not a name list, for two independent reasons:
  #
  #   1. The generated workflows create their own repository when it is missing.
  #      `aws ecr describe-repositories ... || aws ecr create-repository ...`
  #      appears at web/src/components/Sidebar.tsx:817-818 and in three other
  #      generators. ecr:CreateRepository cannot be authorised against a list
  #      that, by definition, does not contain the repository being created, so
  #      an enumeration would deny the very step that exists for the
  #      not-yet-applied case.
  #   2. It keeps the policy a CONSTANT size — three ARNs, six under
  #      cross_account — independent of the caller's services and of the
  #      scheduled-task count. An enumeration grows with the service count and
  #      reaches the 10,240-character inline-policy cap (aws_iam_role_policy is
  #      inline; the 6,144 managed-policy cap does not apply) at around 110
  #      repositories.
  #
  # This is NOT the "defect D1" drift that modules/workloads/ecr.tf:186-193
  # warns against. That warning is about re-deriving a SPECIFIC service's
  # repository name from a template when a resource attribute already holds it,
  # because the two derivations drift per ecr_config mode. Nothing is re-derived
  # here: "${var.project}_service_*" is a namespace, and a prefix cannot drift
  # from an instance of itself — it matches whatever aws_ecr_repository.services
  # names itself, in every mode, including repositories that do not exist yet.
  #
  # The shape is not a new idiom either: it is the iam:PassRole statement below,
  # and modules/workloads/backend.tf:844-847 verbatim.
  #
  # ONE SUPPORTED MODE FALLS OUTSIDE THIS GRANT: ecr_config.mode = "manual_repo".
  # It points a service at an arbitrary registry URI, which reduces to a name
  # like "team/legacy-api" carrying nothing of the project, so no prefix can
  # cover it. No GENERATED workflow is affected — the manual-repo branch
  # (ServiceCICDConfiguration.tsx:143-190) makes no ECR call at all, and the
  # generator builds ECR_REPOSITORY from the project and service name even for
  # the other modes, never from the configured URI. But that generator
  # inconsistency means anyone on this mode already pushes by hand, and a hand
  # push to an in-account ECR repository named outside the convention is now
  # denied where the wildcard allowed it. It fails loudly, at the push step,
  # with AccessDeniedException naming the ARN.
  #
  # THIS account and THIS region, never var.ecr_account_id /
  # var.ecr_account_region — those name repositories that do not exist here, so
  # the grant would silently cover nothing.
  github_ecr_local_repository_arns = [
    for pattern in [
      "${var.project}_backend",
      "${var.project}_service_*",
      "${var.project}_task_*",
    ] :
    "arn:aws:ecr:${var.region}:${var.aws_account_id}:repository/${pattern}"
  ]

  # Unconditional, NOT gated on ecr_strategy. aws_ecr_repository.services
  # (modules/workloads/ecr.tf:231) has no strategy gate — only the backend
  # repository is gated at modules/workloads/ecr.tf:6 — so service repositories
  # exist in this account under cross_account too, and their push would start
  # failing if this list were wrapped in a `local`-strategy condition. A prefix
  # naming an absent repository grants nothing, so the unconditional form costs
  # nothing either.
  #
  # ecr_strategy = "cross_account" adds the SOURCE registry on top. It
  # authorises no generated path: the cross-account workflow is EventBridge-only
  # (ServiceCICDConfiguration.tsx:95-142; its only AWS call is
  # `aws events put-events`). These ARNs exist so a HAND-WRITTEN workflow that
  # reaches the source registry with this role keeps working, which the wildcard
  # allowed until now.
  #
  # This does NOT claim a push into the source account is impossible. An earlier
  # version of this comment said the repository policy there is pull-only, citing
  # AllowCrossAccountPullOnly (modules/workloads/ecr.tf). That is false in the
  # default configuration: that policy is counted on
  # `length(var.ecr_trusted_accounts) > 0`, and with the list empty the source
  # repositories carry default_ecr_policy instead (ecr.tf:32-60), whose principal
  # is `*` and whose actions include ecr:PutImage, DeleteRepository and
  # BatchDeleteImage. So the honest statement is the narrower one: these ARNs are
  # strictly tighter than the `"*"` they replace, and they do not widen anything.
  # The permissiveness of default_ecr_policy is a separate, pre-existing finding
  # and is deliberately not addressed here.
  #
  # ecr_account_region is guarded as well as ecr_account_id: it defaults to ""
  # (variables.tf), and "arn:aws:ecr::<account>:repository/..." with an empty
  # region field is a syntactically valid ARN that matches nothing at all — a
  # grant that looks present in the plan and denies at the call.
  github_ecr_source_repository_arns = (
    var.ecr_strategy == "cross_account" && var.ecr_account_id != "" && var.ecr_account_region != ""
    ? [
      for pattern in [
        "${var.project}_backend",
        "${var.project}_service_*",
        "${var.project}_task_*",
      ] :
      "arn:aws:ecr:${var.ecr_account_region}:${var.ecr_account_id}:repository/${pattern}"
    ]
    : []
  )

  # The assertable surface, and the value the scoped statement below must
  # reference bare. Also an output, so a test may read it without depending on
  # this module being the root module of the run.
  github_ecr_repository_arns = concat(
    local.github_ecr_local_repository_arns,
    local.github_ecr_source_repository_arns,
  )
}

data "aws_iam_policy_document" "github" {
  # Registry-level, and alone in its own statement because there is no resource
  # to scope it to — AWS evaluates ecr:GetAuthorizationToken at the registry, so
  # "*" is the only form it accepts. This wildcard is not a hole: the token it
  # returns is worthless without a repository-level grant, which the next
  # statement supplies and confines to this project. Fold this action into that
  # statement and aws-actions/amazon-ecr-login fails in every generated
  # workflow.
  statement {
    effect    = "Allow"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  # The repository-scoped grant. This is the statement the whole extraction
  # exists to make readable: a test in tests/ can now decode the rendered JSON
  # and check that its Resource list matches this project's repository names and
  # no neighbouring project's, rather than inspecting the local it is built
  # from.
  statement {
    effect    = "Allow"
    actions   = local.github_ecr_scoped_actions
    resources = local.github_ecr_repository_arns
  }

  # UNCHANGED by the ECR scoping fix, minus the nine ECR entries that moved into
  # the two statements above. ecs:* and events:PutEvents are resource-scopable
  # in principle, but narrowing them is a separate change with its own blast
  # radius and is deliberately out of scope here.
  statement {
    effect = "Allow"
    actions = [
      "ecs:UpdateService",
      "ecs:DescribeServices",
      "ecs:RegisterTaskDefinition",
      "ecs:DescribeTaskDefinition",
      "events:PutEvents"
    ]
    resources = ["*"]
  }

  # iam:PassRole is required when calling ecs:UpdateService or ecs:RegisterTaskDefinition
  # because ECS needs to assume the task execution role to pull images from ECR.
  # Scoped to this project's task and execution roles only.
  statement {
    effect = "Allow"
    actions = [
      "iam:PassRole"
    ]
    resources = [
      "arn:aws:iam::${var.aws_account_id}:role/${var.project}_*_task_${var.env}",
      "arn:aws:iam::${var.aws_account_id}:role/${var.project}_*_task_execution_${var.env}",
      "arn:aws:iam::${var.aws_account_id}:role/${var.project}_scheduler_*_task_execution_${var.env}"
    ]
  }
}
