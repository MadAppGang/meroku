# The WIRING between modules/workloads and ../github_policy.
#
# REGRESSION: GithubAccessPolicy granted ECR on Resource "*" — cross-project push. Fixed in /dev:fix session dev-fix-20260909-ecr-scope
#
# The security property itself — that the GitHub Actions deploy role can reach
# this project's ECR repositories and no other project's — is asserted in
# ../../github_policy/tests/ecr_scope.tftest.hcl, against the REAL rendered
# policy, by matching a candidate repository ARN against every Resource granted.
# It cannot be asserted from this directory and this file no longer tries: every
# test file here must declare `mock_data "aws_iam_policy_document"` with a stub
# json string, because a generated mock value is not a policy document and the
# AWS provider validates that CLIENT-side, so without the stub seven resources
# in this module fail with `"policy" contains an invalid JSON policy` before a
# single assertion runs. `module.github_policy.json` therefore returns the stub
# here no matter what the statement blocks say, and `override_data` moves in the
# same direction as the mock rather than out of it. That is precisely why the
# document was extracted into a leaf module.
#
# What is left here is real, and is not covered anywhere else:
#
#   1. The grant is a CONSTANT SIZE. ../github_policy takes no service list at
#      all, so it cannot be asserted there that the size is independent of the
#      service count — the module has no way to be told about services. This
#      module does, and it plans thirty of them.
#      (the_grant_does_not_grow_with_the_service_count)
#   2. The caller passes the right things. ../github_policy declares
#      `project`, `aws_account_id` and `region` as plain string INPUTS, so every
#      assertion over there is downstream of whatever this module hands it. A
#      caller that passes an empty region, or another project's name, renders a
#      perfectly scoped policy for the wrong scope.
#      (the_caller_hands_the_module_this_project_this_account_this_region)
#
# Module OUTPUTS are not mocked — mock_provider replaces provider resources and
# data sources, not locals — so `module.github_policy.ecr_repository_arns` is
# the real value here even though `.json` is not. The leaf suite pins that
# output to the rendered document, which is what makes it a usable proxy.
#
# Run: terraform test  (from modules/workloads)

mock_provider "aws" {
  # Identical to tests/revision_notification.tftest.hcl — see the long comment
  # there for why each entry exists. Briefly: a generated mock string is not a
  # policy document, a CIDR block or an ARN, and the AWS provider validates all
  # three CLIENT-side, before anything would reach a network. Account
  # 000000000000 is synthetic; this repository is public and no real identifier
  # belongs in it.
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }

  mock_data "aws_vpc" {
    defaults = {
      cidr_block = "10.0.0.0/16"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::000000000000:role/mock"
    }
  }
  mock_resource "aws_iam_policy" {
    defaults = {
      arn = "arn:aws:iam::000000000000:policy/mock"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock"
    }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = {
      arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock"
    }
  }
  mock_resource "aws_lambda_function" {
    defaults = {
      arn = "arn:aws:lambda:us-east-1:000000000000:function:mock"
    }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = {
      arn = "arn:aws:events:us-east-1:000000000000:rule/mock"
    }
  }
}

# null_resource.build_ci_lambda (lambda.tf) and the random_* resources. Nothing
# here is asserted on; they are mocked only so the suite needs no provider that
# reaches out.
mock_provider "null" {}
mock_provider "random" {}

# The smallest input set modules/workloads accepts. Every value is synthetic.
variables {
  project = "acme"
  env     = "dev"

  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"

  # Required with no default ON PURPOSE (variables.tf:96): an omitted value must
  # fail rather than fall back to a wildcard that lets any repository assume the
  # deploy role. The trust side of this role is already correctly scoped; this
  # file is about the PERMISSION side.
  github_subjects = ["repo:acme/acme:ref:refs/heads/main"]

  services = [
    { name = "orders", auto_deploy = true },
    { name = "billing", auto_deploy = false },
    { name = "search" },
  ]
}

# ---------------------------------------------------------------------------
# 1. What this module hands the policy module.
#
# ../github_policy renders a correct policy for whatever project, account and
# region it is given, and its own tests can only ever check it against the
# values they pass in. The scope is only this project's scope because THIS file's
# module block passes var.project, local.aws_account_id and
# data.aws_region.current.name. Nothing downstream can notice if it stops.
# ---------------------------------------------------------------------------

run "the_caller_hands_the_module_this_project_this_account_this_region" {
  command = plan

  variables {
    # Deliberately NOT the "acme" the rest of this file uses, and deliberately
    # not a name anybody would reach for when hardcoding one.
    #
    # `project` reaches ../github_policy as a plain string. If the module block
    # passed a LITERAL instead of var.project, an assertion planned with
    # project = "acme" against a literal "acme" would agree with itself and
    # report success — which is precisely the shape of assertion this whole
    # session was called in to stop writing. Planning with a project name that
    # appears nowhere in modules/ is what makes the assertion below capable of
    # failing.
    project = "zeta"
  }

  # Every granted ARN carries this project's prefix. Not the security property —
  # `repository/acme*` would satisfy this and reach a project called `acmecorp`,
  # which is why the real assertion lives in the leaf module's suite and matches
  # rather than inspects — but it does pin the one thing that suite cannot see:
  # that the project name reaching the module is var.project and not a literal
  # or another variable.
  assert {
    condition = alltrue([
      for arn in module.github_policy.ecr_repository_arns :
      strcontains(arn, ":repository/${var.project}")
    ])
    error_message = "Every ECR ARN the deploy role is granted must carry THIS project's name, and the name has to arrive from var.project. ../github_policy renders an equally well-formed policy for any project it is given, so a caller that passes a literal, or another module's name, produces a policy that is perfectly scoped to the wrong project — and every assertion in that module's own test suite still passes, because they pass the same wrong value in. Got: ${jsonencode(module.github_policy.ecr_repository_arns)}"
  }

  # The account and region halves of the ARN are as much a part of the scope as
  # the repository name, and they are the halves the leaf module cannot check:
  # it takes both as plain string inputs, deliberately, so that `terraform test`
  # there needs no credentials. An ARN with an empty region or account field —
  # `arn:aws:ecr::000000000000:repository/acme_backend` — is syntactically
  # valid, shows in the plan diff as a real grant, and matches nothing at all,
  # so it denies every push while looking present to everyone reading the plan.
  # Passing var.ecr_account_region (default "") instead of the current region is
  # a one-word edit that produces exactly that.
  assert {
    condition = alltrue([
      for arn in module.github_policy.ecr_repository_arns :
      length(split(":", arn)) == 6 && split(":", arn)[3] != "" && split(":", arn)[4] != ""
    ])
    error_message = "Every ECR ARN granted must name a non-empty region and a non-empty account. Both are passed IN to ../github_policy — from data.aws_region.current.name and local.aws_account_id — so the module itself cannot tell whether the caller supplied them. An empty field yields a syntactically valid ARN that IAM accepts, that a plan diff renders as a grant, and that matches no repository in any account, which denies every push in the project and gives no clue why. Got: ${jsonencode(module.github_policy.ecr_repository_arns)}"
  }
}

# ---------------------------------------------------------------------------
# 2. The grant is a namespace, not an enumeration.
#
# Thirty services, and the policy is still three ARNs. This cannot be asserted
# in ../github_policy's own suite: that module takes no service list, which is
# the structural half of the same guarantee, and this run is the behavioural
# half — it plans the caller that DOES have the list and shows nothing flows
# from it into the grant.
#
# Two things break the day the grant becomes a per-repository enumeration, and
# neither shows up at plan or apply time:
#
#   - ecr:CreateRepository stops working. The generated workflows run
#     `aws ecr describe-repositories || aws ecr create-repository`
#     (web/src/components/Sidebar.tsx:817-818), so the grant has to cover a
#     repository that does not exist yet — which an enumeration, by definition,
#     cannot.
#   - aws_iam_role_policy is an INLINE policy, capped at 10,240 characters.
#     A per-repository list reaches it at around 110 repositories, and the
#     failure arrives on somebody's apply, not here.
# ---------------------------------------------------------------------------

run "the_grant_does_not_grow_with_the_service_count" {
  command = plan

  variables {
    services = [for i in range(30) : { name = "svc${i}" }]
  }

  assert {
    condition     = length(module.github_policy.ecr_repository_arns) == 3
    error_message = "The ECR grant must stay three ARNs — the backend, service and scheduled-task name PREFIXES — no matter how many services the project has. Thirty services here and a grant that is not three means the prefixes were replaced by an enumeration of repository names, which denies `create-repository` for every repository not yet applied and walks the inline policy into its 10,240-character cap at around 110 repositories. Both failures land on a user's apply or in a user's pipeline, never here. Got ${length(module.github_policy.ecr_repository_arns)}: ${jsonencode(module.github_policy.ecr_repository_arns)}"
  }
}
