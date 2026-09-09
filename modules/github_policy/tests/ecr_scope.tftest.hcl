# What the GitHub Actions deploy role can actually reach in ECR, asserted
# against the REAL rendered policy — the string aws_iam_role_policy.github_access
# hands to IAM.
#
# REGRESSION: GithubAccessPolicy granted ECR on Resource "*" — cross-project push. Fixed in /dev:fix session dev-fix-20260909-ecr-scope
#
# The requirement, in one sentence: this role must do everything its CI pipeline
# needs against ITS OWN ECR repositories, and nothing at all against another
# meroku project's repositories in the same AWS account. Two meroku projects
# sharing one account is the normal case, not a contrived one — the account in
# ai-docs/sessions/dev-fix-20260909-ecr-scope/real-validation.md hosts two.
#
# The contract, in the order the contract matters.
#
#   1. The pipeline still works. Every ECR call a generated workflow makes, on
#      every repository this project creates — including one that does not exist
#      yet, which is the `create-repository` case.
#      (the_pipeline_reaches_every_repository_this_project_creates)
#   2. A neighbouring project is unreachable. Not "the ARN string looks scoped" —
#      unreachable, by matching its repository ARN against every Resource
#      granted.
#      (a_neighbouring_project_is_unreachable)
#   3. The grant is the WHOLE grant: one repository-level ECR statement, an exact
#      action set, every Effect an Allow.
#      (the_rendered_policy_is_the_whole_grant)
#   4. Adding the cross-account source registry widens nothing, and a
#      half-configured cross_account emits no ARN that matches nothing.
#      (cross_account_*)
#
# ---------------------------------------------------------------------------
# WHY THE ASSERTIONS MATCH INSTEAD OF INSPECTING STRINGS
#
# The predecessor of this file lived in modules/workloads/tests/ and asserted
# `startswith(arn, "arn:aws:ecr:") && strcontains(arn, ":repository/acme")` on a
# local, because the rendered json was unreadable there (see ../main.tf for why
# the module was extracted). Both halves of that condition hold for
#
#     arn:aws:ecr:us-east-1:000000000000:repository/acme*
#
# which reaches a DIFFERENT project called `acmecorp` in the same account, and
# both hold for
#
#     arn:aws:ecr::000000000000:repository/acme_backend
#
# which reaches nothing at all and breaks every push. A shape check cannot tell
# either from the correct value; a matcher can, and ./matcher is that matcher.
# It turns each granted Resource into the glob IAM treats it as and asks the
# only question the requirement asks: given this repository, what can this role
# do to it?
#
# Fixtures are real names. `circl_*` and the unprefixed `backend` are
# repositories that exist in the account real-validation.md examined; `acmecorp`
# is the prefix-one-character-too-loose case, which is the mistake this file
# exists to catch and which no previous test could see.
#
# Run: terraform test  (from modules/github_policy)
# ---------------------------------------------------------------------------

# The real AWS provider, and no credentials anywhere.
#
# aws_iam_policy_document is a CLIENT-SIDE renderer — the provider walks the
# statement blocks and emits JSON locally — and this module declares no data
# source that reads AWS, which is a documented constraint of ../variables.tf
# rather than an accident. So the three skips below are enough: nothing here
# resolves a credential, calls STS, or touches the instance metadata endpoint.
# That is the entire reason the policy was extracted into a module of its own.
provider "aws" {
  region = "us-east-1"

  access_key                  = "AKIAIOSFODNN7EXAMPLE"
  secret_key                  = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
}

variables {
  # THE variable under test. The prefix it contributes is the only thing
  # separating this project's ECR namespace from a neighbour's in a shared
  # account.
  project = "acme"
  env     = "dev"

  # Synthetic. This repository is public (CLAUDE.md, line 1): no real AWS
  # account ID, ARN, key or credential may ever appear in a committed file,
  # test fixtures included. The key pair above is AWS's own documentation
  # example.
  aws_account_id = "000000000000"
  region         = "us-east-1"
}

# ---------------------------------------------------------------------------
# 1. The document itself: one grant, exactly these actions, all of them Allows.
#
# This run reads the rendered JSON directly. Everything it asserts is a property
# of the WHOLE document rather than of one candidate resource, which is what the
# matcher runs below cannot see — a matcher only ever reports reach, and reach
# is identical whether a grant is made once or three times.
# ---------------------------------------------------------------------------

run "the_rendered_policy_is_the_whole_grant" {
  # The ECR calls every generated workflow makes, and no others.
  #
  #   describe-or-create probe  DescribeRepositories, CreateRepository
  #                             (web/src/components/Sidebar.tsx:817-818)
  #   layer upload              InitiateLayerUpload, UploadLayerPart,
  #                             CompleteLayerUpload, BatchCheckLayerAvailability
  #   manifest                  PutImage, BatchGetImage
  #
  # EQUALITY, not containment, and that is the point of the assertion. The
  # predecessor test asserted these eight were PRESENT, so adding
  # ecr:DeleteRepository and ecr:BatchDeleteImage alongside them passed — which
  # hands the CI role the ability to delete this project's images and
  # repositories outright, a far worse outcome than the over-wide grant this
  # whole change removed. Whoever adds a ninth action has to come here and say
  # which pipeline step needs it.
  assert {
    condition = jsonencode(sort(flatten([
      for s in jsondecode(output.json).Statement : flatten([s.Action])
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
      ]))) == jsonencode([
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:CompleteLayerUpload",
      "ecr:CreateRepository",
      "ecr:DescribeRepositories",
      "ecr:InitiateLayerUpload",
      "ecr:PutImage",
      "ecr:UploadLayerPart",
    ])
    error_message = "The repository-scoped statement must grant EXACTLY the eight ECR calls the generated workflows make — no fewer, no more. One missing is an AccessDeniedException part-way through a `docker push` in a pipeline nobody is watching, since the policy just gets smaller and plan and apply both stay green. One extra is reach nobody asked for: ecr:DeleteRepository or ecr:BatchDeleteImage here lets a compromised or merely buggy workflow destroy this project's images and repositories, which no CI step needs and no rollback recovers. Rendered policy was:\n${output.json}"
  }

  # Exactly ONE statement carries those actions. This is the assertion that
  # kills the reversion the previous suites could not see: leave the scoped
  # statement exactly as shipped and add a SECOND statement granting
  # ecr:PutImage and the layer-upload calls on resources = ["*"], and the
  # original account-wide push is restored in full while every assertion about
  # the scoped statement still holds. The grant has to be checked as a whole
  # document, and until this module existed there was nowhere to do that from.
  assert {
    condition = length([
      for s in jsondecode(output.json).Statement : s
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ]) == 1
    error_message = "Repository-level ECR actions must be granted by exactly ONE statement. A second one — most plausibly written when a deploy fails with AccessDenied, with the action names inlined and a wildcard resource — restores the cross-project push in full and leaves every property of the first statement intact, so nothing that inspects only the scoped statement can see it. Rendered policy was:\n${output.json}"
  }

  # ecr:GetAuthorizationToken, alone, on "*".
  #
  # IAM evaluates it at the REGISTRY, so there is no ARN to name and "*" is the
  # only form that works. That is not a hole: the token it returns is worthless
  # without a repository-level grant, which the statement above confines to this
  # project. The failure this guards is the careless "tighten everything" fix —
  # fold this action in with the repository ARNs and it is denied,
  # aws-actions/amazon-ecr-login fails, and no image can be pushed anywhere, in
  # any project, from the first apply after the change.
  assert {
    condition = jsonencode([
      for s in jsondecode(output.json).Statement : sort(flatten([s.Resource]))
      if contains(flatten([s.Action]), "ecr:GetAuthorizationToken")
    ]) == jsonencode([["*"]])
    error_message = "ecr:GetAuthorizationToken must be granted exactly once, on Resource \"*\". Scoping it to repository ARNs it can never match denies it, which breaks `docker login` in every workflow meroku generates and therefore every image push in every project — the mirror-image failure of the defect this module exists to fix, and a worse one, because it breaks projects that were working. Rendered policy was:\n${output.json}"
  }

  assert {
    condition = !contains(flatten([
      for s in jsondecode(output.json).Statement : flatten([s.Action])
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ]), "ecr:GetAuthorizationToken")
    error_message = "ecr:GetAuthorizationToken has been folded into the repository-scoped statement. It is registry-level: pairing it with repository ARNs denies it outright, and `docker login` — the first ECR step of every generated workflow — starts failing everywhere. It needs its own statement, and it has one. Rendered policy was:\n${output.json}"
  }

  # Every statement is an Allow.
  #
  # Nothing before this asserted it, and the consequence of a flip is not a
  # narrower role: an explicit Deny beats every Allow in IAM and cannot be
  # overridden by any other policy, so a one-word edit here stops every push in
  # every project permanently, and stops it in a way no additional grant
  # anywhere can repair. It is also invisible to the matcher in ./matcher, which
  # deliberately does not model Deny precedence.
  assert {
    condition     = alltrue([for s in jsondecode(output.json).Statement : s.Effect == "Allow"])
    error_message = "Every statement in this policy must have Effect \"Allow\". A Deny here is not a tightening — IAM gives explicit Deny absolute precedence over every Allow in every attached policy, so it revokes the affected calls for this role permanently and unrecoverably. Rendered policy was:\n${output.json}"
  }

  # Three ARNs, and three regardless of how many services or scheduled tasks the
  # project has — this module takes no service list at all, which is the
  # structural half of the same guarantee (modules/workloads/tests/
  # github_policy_scope.tftest.hcl plans a caller with thirty services and
  # asserts the number is still three).
  #
  # The count is not decoration. A per-repository enumeration would break
  # ecr:CreateRepository, which by definition cannot be authorised against a
  # list that excludes the repository being created, and would grow into the
  # 10,240-character inline-policy cap at around 110 repositories.
  assert {
    condition = length(one([
      for s in jsondecode(output.json).Statement : flatten([s.Resource])
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ])) == 3
    error_message = "The repository-scoped grant must be exactly three ARNs — this project's backend repository, its service repositories and its scheduled-task repositories, as PREFIXES. More usually means an enumeration crept in, which denies `create-repository` for a repository that does not exist yet and grows with the service count; fewer means a whole class of push was silently revoked. Rendered policy was:\n${output.json}"
  }

  # The two exported values are the ones modules/workloads/tests/ can still read
  # under its mock provider, where the rendered json cannot be. Pinning them to
  # the document here is what makes an assertion over there mean anything.
  assert {
    condition = jsonencode(sort(output.ecr_repository_arns)) == jsonencode(one([
      for s in jsondecode(output.json).Statement : sort(flatten([s.Resource]))
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ]))
    error_message = "output.ecr_repository_arns must be exactly what the repository-scoped statement grants. It is the only part of this policy modules/workloads/tests/ can read — the rendered json is mocked there — so if the two drift, the assertions in that directory describe a value the role never receives. Got ${jsonencode(sort(output.ecr_repository_arns))} against the rendered policy:\n${output.json}"
  }

  assert {
    condition = jsonencode(sort(output.ecr_scoped_actions)) == jsonencode(sort(one([
      for s in jsondecode(output.json).Statement : flatten([s.Action])
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ])))
    error_message = "output.ecr_scoped_actions must be exactly the action set the repository-scoped statement grants. The matcher runs below compare a repository's reach against this output rather than against a copied literal, so a drift between the two would make every one of those comparisons compare a value to itself. Got ${jsonencode(sort(output.ecr_scoped_actions))} against the rendered policy:\n${output.json}"
  }
}

# ---------------------------------------------------------------------------
# 2. The pipeline still works — by matching, on every repository this project
#    creates.
#
# The names are the three templates that create the repositories:
# modules/workloads/ecr.tf (backend, services) and modules/ecs_task/main.tf
# (scheduled tasks). `acme_service_brandnew` deliberately does not exist: the
# generated workflow runs `describe-repositories || create-repository`, so the
# grant has to cover a repository nobody has applied yet. That is why the policy
# is written as prefixes and not as a list of names, and it is the one property
# an enumeration cannot have.
# ---------------------------------------------------------------------------

run "the_pipeline_reaches_every_repository_this_project_creates" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.the_rendered_policy_is_the_whole_grant.json

    candidates = {
      # ${project}_backend — modules/workloads/ecr.tf:5
      acme_backend = "arn:aws:ecr:us-east-1:000000000000:repository/acme_backend"

      # ${project}_service_${name} — modules/workloads/ecr.tf:234. The hyphen in
      # the second one is not decoration: hyphens are legal in ECR names and in
      # meroku service names (real-validation.md lists
      # coretechx_service_magento-bridge), and a matcher that escaped its glob
      # carelessly would treat it as a metacharacter.
      acme_service_api            = "arn:aws:ecr:us-east-1:000000000000:repository/acme_service_api"
      acme_service_magento_bridge = "arn:aws:ecr:us-east-1:000000000000:repository/acme_service_magento-bridge"

      # ${project}_task_${name} — modules/ecs_task/main.tf:62 and
      # modules/event_bridge_task/ecs.tf:4.
      acme_task_cleanup = "arn:aws:ecr:us-east-1:000000000000:repository/acme_task_cleanup"

      # DOES NOT EXIST. The create-repository case: the workflow's
      # `aws ecr describe-repositories || aws ecr create-repository` has to be
      # authorised for a name no applied resource has yet produced.
      acme_service_brandnew = "arn:aws:ecr:us-east-1:000000000000:repository/acme_service_brandnew"
    }
  }

  # Every repository, every call. Compared against the module's own exported
  # action set rather than a literal so that this run tests MATCHING and run 1
  # tests the SET — otherwise a wrong action list would have to be corrected in
  # two files and the second correction would look like a test fix.
  assert {
    condition = alltrue([
      for label, actions in output.ecr_repository_reach :
      jsonencode(actions) == jsonencode(sort(run.the_rendered_policy_is_the_whole_grant.ecr_scoped_actions))
    ])
    error_message = "Every repository this project creates must be reachable for the whole push path. A repository missing an action here fails part-way through a deploy — after the image is built, in the middle of a layer upload — and a repository reachable for nothing at all means a name pattern was dropped, which silently revokes a whole class of push: scheduled tasks, or services, or the backend. `acme_service_brandnew` must be reachable too; it does not exist, and the generated workflow creates it. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

# ---------------------------------------------------------------------------
# 2b. The adjacent grant in the same document, checked with the same machinery.
#
# iam:PassRole is required by ecs:UpdateService and ecs:RegisterTaskDefinition —
# ECS assumes the task execution role to pull the image — and it is scoped by
# the same prefix reasoning as ECR. It is asserted here because the failure has
# the same two shapes: too tight and every deploy fails at the step AFTER the
# push, with the image already in ECR; too loose and this role can hand a
# neighbouring project's execution role to ECS, starting a task that holds that
# project's permissions.
# ---------------------------------------------------------------------------

run "the_passrole_grant_is_scoped_to_this_project_too" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.the_rendered_policy_is_the_whole_grant.json

    candidates = {
      # This project's roles — modules/workloads/backend.tf and
      # modules/ecs_task/main.tf name them (CLAUDE.md's naming table).
      own_backend_task      = "arn:aws:iam::000000000000:role/acme_backend_task_dev"
      own_backend_execution = "arn:aws:iam::000000000000:role/acme_backend_task_execution_dev"
      own_scheduler         = "arn:aws:iam::000000000000:role/acme_scheduler_cleanup_task_execution_dev"

      # A neighbouring project's, and this project's in the WRONG environment.
      # The env suffix is part of the scope: prod's execution role must not be
      # passable from dev's CI.
      neighbour_task = "arn:aws:iam::000000000000:role/circl_backend_task_dev"
      acmecorp_task  = "arn:aws:iam::000000000000:role/acmecorp_backend_task_dev"
      wrong_env      = "arn:aws:iam::000000000000:role/acme_backend_task_prod"
    }
  }

  assert {
    condition = alltrue([
      for label in ["own_backend_task", "own_backend_execution", "own_scheduler"] :
      contains(output.reach[label], "iam:PassRole")
    ])
    error_message = "This project's own ECS task and execution roles must be passable. Without iam:PassRole on them, ecs:RegisterTaskDefinition and ecs:UpdateService both fail and no deploy completes — after every ECR step has already succeeded and the image is sitting in the registry, which is the most confusing point in the pipeline to fail at. Reach was: ${jsonencode(output.reach)}"
  }

  assert {
    condition = alltrue([
      for label in ["neighbour_task", "acmecorp_task", "wrong_env"] :
      !contains(output.reach[label], "iam:PassRole")
    ])
    error_message = "No role outside this project and this environment may be passable. Handing another project's task execution role to ECS starts a task holding that project's permissions — a privilege escalation straight across the boundary the ECR scoping draws — and `wrong_env` is the same escalation into production from a dev pipeline. Reach was: ${jsonencode(output.reach)}"
  }
}

# ---------------------------------------------------------------------------
# 3. THE requirement. A neighbouring project is unreachable.
#
# Nothing before this file tested it. Every assertion in the predecessor suites
# was a shape check on a string, and no second project's name appeared in either
# of them; the only evidence that ever existed is one manual
# iam:SimulateCustomPolicy run recorded in real-validation.md, which is not in
# CI and will not run again. This run is that simulation, in CI, on every plan.
# ---------------------------------------------------------------------------

run "a_neighbouring_project_is_unreachable" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.the_rendered_policy_is_the_whole_grant.json

    candidates = {
      # A second meroku project in the same account. These are real repository
      # names from the account real-validation.md examined, which hosts two
      # meroku projects side by side — the situation the requirement is about.
      circl_backend     = "arn:aws:ecr:us-east-1:000000000000:repository/circl_backend"
      circl_service_api = "arn:aws:ecr:us-east-1:000000000000:repository/circl_service_api"
      circl_task_sync   = "arn:aws:ecr:us-east-1:000000000000:repository/circl_task_sync"

      # Real, and unprefixed: the account has a bare `backend` repository
      # belonging to no meroku project (real-validation.md:83). It must not be
      # reachable, and a grant loose enough to reach it — `repository/*backend`,
      # say — reaches every other project's backend as well.
      unprefixed_backend = "arn:aws:ecr:us-east-1:000000000000:repository/backend"

      # THE case the whole exercise is about. `acmecorp` is a different project
      # whose name merely starts with this one's. A prefix written without its
      # separator — `${var.project}*` instead of `${var.project}_*` — is one
      # character's difference in the source, produces ARNs that pass every
      # shape check ever written against this policy, and hands `acme`'s CI role
      # write access to `acmecorp`'s entire registry.
      acmecorp_backend     = "arn:aws:ecr:us-east-1:000000000000:repository/acmecorp_backend"
      acmecorp_service_api = "arn:aws:ecr:us-east-1:000000000000:repository/acmecorp_service_api"
      acmecorp_task_sync   = "arn:aws:ecr:us-east-1:000000000000:repository/acmecorp_task_sync"

      # This project's own repository name, in the wrong REGION and in the wrong
      # ACCOUNT. Both must be unreachable, because both fields of the granted
      # ARN are part of the scope: a wildcarded region reaches this project's
      # other environments, and a wildcarded account reaches every account the
      # role is ever used against.
      acme_backend_elsewhere_region  = "arn:aws:ecr:eu-west-1:000000000000:repository/acme_backend"
      acme_backend_elsewhere_account = "arn:aws:ecr:us-east-1:111111111111:repository/acme_backend"
    }
  }

  assert {
    condition     = alltrue([for label, actions in output.ecr_repository_reach : length(actions) == 0])
    error_message = "No repository outside this project's namespace may be reachable for ANY repository-level ECR action. This is the defect the whole change exists to remove: with the old wildcard, project `acme`'s CI role could push an image over project `circl`'s tags or create repositories inside `circl`'s namespace, and nothing anywhere reported it — the deploy succeeded either way, which is why it survived to v4.7.0. `acmecorp_*` is the same hole one character wide: a prefix written without its separator reaches every project whose name merely starts with this one's. The two `_elsewhere_` entries are this project's own repository name in another region and another account; reaching them means the region or account field of the grant was wildcarded or emptied, not that the repository name pattern was wrong. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

# ---------------------------------------------------------------------------
# 3b. The property is directional, so it is asserted in both directions.
#
# Run 3 shows `acme` cannot reach `acmecorp`. That leaves the other half:
# `acmecorp`, deployed from its own environment, must not reach `acme`. A
# suffix-shaped mistake — matching on `*${var.project}_*` — passes run 3 and
# fails here.
# ---------------------------------------------------------------------------

run "the_longer_project_name_renders_its_own_scope" {
  variables {
    project = "acmecorp"
  }
}

run "the_longer_project_cannot_reach_the_shorter_one" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.the_longer_project_name_renders_its_own_scope.json

    candidates = {
      acme_backend     = "arn:aws:ecr:us-east-1:000000000000:repository/acme_backend"
      acme_service_api = "arn:aws:ecr:us-east-1:000000000000:repository/acme_service_api"
      acme_task_sync   = "arn:aws:ecr:us-east-1:000000000000:repository/acme_task_sync"
    }
  }

  assert {
    condition     = alltrue([for label, actions in output.ecr_repository_reach : length(actions) == 0])
    error_message = "Project `acmecorp` must not reach project `acme`'s repositories. The containment relation between the two names runs both ways, and so must the test: a pattern anchored only at its end — `*acme_backend` — passes the shorter-reaches-longer check and still lets the longer project write over the shorter one's images. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }

  assert {
    condition     = length(run.the_longer_project_name_renders_its_own_scope.ecr_repository_arns) == 3
    error_message = "The grant is three ARNs for every project, whatever the project is called. Got ${jsonencode(run.the_longer_project_name_renders_its_own_scope.ecr_repository_arns)}"
  }
}

# ---------------------------------------------------------------------------
# 4. cross_account.
#
# In this mode the generated workflow is EventBridge-only and pushes nothing, so
# the source-account ARNs authorise no generated path. They exist so a
# HAND-WRITTEN workflow that pulls from the source registry with this role keeps
# working — the old wildcard allowed that, and a fix that quietly took it away
# would be a regression nobody could see from a plan.
#
# What must not happen is that the source registry is added by WIDENING. That
# registry is shared between environments by design, so a wildcard there lets
# one environment's CI role overwrite the images every other environment
# deploys from — a larger blast radius than the single-account case, not a
# smaller one.
# ---------------------------------------------------------------------------

run "cross_account_renders_both_registries" {
  variables {
    ecr_strategy       = "cross_account"
    ecr_account_id     = "999999999999"
    ecr_account_region = "eu-west-1"
  }

  assert {
    condition = length(one([
      for s in jsondecode(output.json).Statement : flatten([s.Resource])
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ])) == 6
    error_message = "Under ecr_strategy = \"cross_account\" the repository-scoped grant must be six ARNs: this project's three name patterns in THIS account, plus the same three in the source account. Fewer means a working path was revoked — dropping the local three breaks pushes to service repositories, which exist in this mode too because aws_ecr_repository.services has no ecr_strategy gate; dropping the source three breaks any hand-written workflow that pulls from the source registry with this role. More means something other than this project's namespace was granted. Rendered policy was:\n${output.json}"
  }

  # Still one statement, still exactly the eight actions. Adding an account is
  # the change most likely to reintroduce a wildcard — "we do not know what the
  # source account names its repositories, so grant the registry" — and it would
  # arrive as a second statement rather than as a widening of this one.
  assert {
    condition = length([
      for s in jsondecode(output.json).Statement : s
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ]) == 1
    error_message = "Cross-account mode must add ARNs to the existing statement, not a second statement. A separate statement for the source registry is where a wildcard resource gets written when nobody knows what the source account calls its repositories, and that wildcard reaches the local account too. Rendered policy was:\n${output.json}"
  }
}

run "cross_account_reaches_both_registries_and_neither_neighbour" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.cross_account_renders_both_registries.json

    candidates = {
      # This project, in this account — must still work. aws_ecr_repository.services
      # has no ecr_strategy gate, so these repositories exist in this mode too.
      local_backend     = "arn:aws:ecr:us-east-1:000000000000:repository/acme_backend"
      local_service_api = "arn:aws:ecr:us-east-1:000000000000:repository/acme_service_api"
      local_task_sync   = "arn:aws:ecr:us-east-1:000000000000:repository/acme_task_sync"

      # This project, in the source account and the source region — the pull a
      # hand-written cross-account workflow makes.
      source_backend     = "arn:aws:ecr:eu-west-1:999999999999:repository/acme_backend"
      source_service_api = "arn:aws:ecr:eu-west-1:999999999999:repository/acme_service_api"
      source_task_sync   = "arn:aws:ecr:eu-west-1:999999999999:repository/acme_task_sync"
    }
  }

  assert {
    condition = alltrue([
      for label, actions in output.ecr_repository_reach :
      jsonencode(actions) == jsonencode(sort(run.cross_account_renders_both_registries.ecr_scoped_actions))
    ])
    error_message = "In cross_account mode this project's repositories must be reachable in BOTH registries: locally, because service and scheduled-task repositories are created here regardless of strategy and their pushes must keep working, and in the source account, because that is the pull the mode exists for. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

run "cross_account_does_not_widen_to_a_neighbour" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.cross_account_renders_both_registries.json

    candidates = {
      local_circl         = "arn:aws:ecr:us-east-1:000000000000:repository/circl_backend"
      local_acmecorp      = "arn:aws:ecr:us-east-1:000000000000:repository/acmecorp_backend"
      local_unprefixed    = "arn:aws:ecr:us-east-1:000000000000:repository/backend"
      source_circl        = "arn:aws:ecr:eu-west-1:999999999999:repository/circl_backend"
      source_acmecorp     = "arn:aws:ecr:eu-west-1:999999999999:repository/acmecorp_backend"
      source_unprefixed   = "arn:aws:ecr:eu-west-1:999999999999:repository/backend"
      source_wrong_region = "arn:aws:ecr:us-east-1:999999999999:repository/acme_backend"
    }
  }

  assert {
    condition     = alltrue([for label, actions in output.ecr_repository_reach : length(actions) == 0])
    error_message = "Adding the cross-account source registry must not widen the grant in either account. The source registry is SHARED between environments by design, so a wildcard there lets one environment's CI role overwrite the images every other environment deploys from — a bigger blast radius than the single-account defect, not a smaller one. `source_wrong_region` is this project's own name in the source account but the WRONG region, which must also be unreachable: the source region is configured, not wildcarded. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

# ---------------------------------------------------------------------------
# 5. The half-configured cross_account, which is the trap nobody was watching.
#
# ecr_account_region defaults to "" (../variables.tf). Building the source ARNs
# without checking it produces
#
#     arn:aws:ecr::999999999999:repository/acme_backend
#
# — a SYNTACTICALLY VALID ARN with an empty region field, which IAM accepts,
# which shows up in a plan diff as a real grant, and which matches nothing at
# all. ../main.tf guards against it in a comment and a condition; until this run
# existed, nothing tested that the guard was still there, and every assertion
# ever written about this policy (`startswith(arn, "arn:aws:ecr:")`,
# `strcontains(arn, ":repository/acme")`) passes on that string.
#
# The failure it causes is worse than a denied pull, because the same guard
# covers both halves of the concat: drop it and the LOCAL push path is fine but
# the plan carries three ARNs that grant nothing, and the next person to widen
# the policy does so from a document they cannot read.
# ---------------------------------------------------------------------------

run "cross_account_without_a_region_emits_no_source_arns" {
  variables {
    ecr_strategy       = "cross_account"
    ecr_account_id     = "999999999999"
    ecr_account_region = "" # the default, and the trap
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "cross_account with no ecr_account_region must fall back to this account's three ARNs and emit no source-registry ARNs at all. An ARN built with an empty region field — arn:aws:ecr::999999999999:repository/acme_backend — is syntactically valid, is accepted by IAM, appears in the plan as a grant, and matches nothing. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

run "no_arn_with_an_empty_field_is_ever_emitted" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.cross_account_without_a_region_emits_no_source_arns.json

    # Not used by the assertion below, but the module requires a value and an
    # empty map would make the failure message read as if nothing was checked.
    candidates = {
      acme_backend = "arn:aws:ecr:us-east-1:000000000000:repository/acme_backend"
    }
  }

  assert {
    condition     = length(output.malformed_ecr_resources) == 0
    error_message = "The policy grants an ECR ARN with an empty region, account or resource field. Such an ARN is accepted by IAM, is visible in the plan diff as a grant, and matches nothing — so the permission looks present to everyone reading the plan and denies at the call, which is the hardest kind of ECR failure to diagnose. Offending resources: ${jsonencode(output.malformed_ecr_resources)}"
  }

  # The local push path is untouched by the missing region. Stated separately
  # because the guard covers both halves of the concat, and a guard written too
  # broadly would suppress the LOCAL ARNs as well and break every push while
  # passing the assertion above.
  assert {
    condition     = length(output.ecr_repository_reach["acme_backend"]) > 0
    error_message = "A half-configured cross_account must not disturb the local push path. The guard that suppresses the source-registry ARNs when ecr_account_region is empty has to suppress ONLY those; written a shade too broadly it takes this account's three ARNs with it and denies every push in the project. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

run "cross_account_without_an_account_id_emits_no_source_arns" {
  variables {
    ecr_strategy       = "cross_account"
    ecr_account_id     = "" # the default
    ecr_account_region = "us-east-1"
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "cross_account with no ecr_account_id must emit no source-registry ARNs. arn:aws:ecr:us-east-1::repository/acme_backend has an empty account field: valid syntax, real-looking plan diff, matches nothing. The account half is guarded for the same reason as the region half and has to stay guarded with it. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

# The mode is `local` and the two cross-account inputs are set anyway — a
# leftover from a project that was switched back, or a YAML that carries both.
# The source registry must not be granted on the strength of the account ID
# alone: strategy is what decides.
run "local_strategy_ignores_a_stray_source_account" {
  variables {
    ecr_strategy       = "local"
    ecr_account_id     = "999999999999"
    ecr_account_region = "eu-west-1"
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "Under ecr_strategy = \"local\" the source registry must not be granted, even when ecr_account_id and ecr_account_region still carry values — switching a project back to local ECR leaves those fields populated in its YAML, and a grant that keys off them rather than off the strategy quietly keeps write access to a registry the project no longer uses. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}
