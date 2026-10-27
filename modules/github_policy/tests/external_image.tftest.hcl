# REGRESSION: MTL-288 GitHub Actions role cannot push to an external backend image repository — Fixed in /dev:fix session dev-fix-20261028-002336-a4ff689a
#
# An opted-in external backend image repository, asserted against the REAL
# rendered policy.
#
# workload.backend_external_docker_image can name an ECR repository in THIS
# account whose name carries nothing of the project (`shared/acme-api`). The
# three prefixes in ../main.tf cannot cover it, so before this change every ECR
# call CI made on it was denied. The fix is opt-in: the image reaches this
# module as `backend_external_image`, and its repository is added to the
# EXISTING scoped statement only when `push_external_image` is true AND the
# image is an ECR URI in this account and this region.
#
# The contract:
#
#   1. Flag on, same account and region: the three prefixes are kept and
#      exactly ONE extra ARN is granted — the external repository, by exact
#      name, in the one existing repository-scoped statement.
#   2. Flag off, another account, another region, a non-ECR image, an empty
#      value: nothing is added.
#   3. Nothing in the image can widen the grant: a wildcard in the name is not
#      parsed, a digest is stripped, and an image naming one of this project's
#      own repositories adds no duplicate.
#   4. ecr:GetDownloadUrlForLayer is in the scoped statement (cache-from,
#      FROM <ecr image>).
#
# Run: terraform test  (from modules/github_policy)

# Identical to ecr_scope.tftest.hcl: the real AWS provider, no credentials.
# aws_iam_policy_document renders client-side and this module reads nothing
# from AWS.
provider "aws" {
  region = "us-east-1"

  access_key                  = "AKIAIOSFODNN7EXAMPLE"
  secret_key                  = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
}

variables {
  # Synthetic. This repository is public: no real account ID may appear here.
  project        = "acme"
  env            = "dev"
  aws_account_id = "000000000000"
  region         = "us-east-1"
}

# ---------------------------------------------------------------------------
# (a) + (i) Flag on, same account, same region.
# ---------------------------------------------------------------------------

run "flag_on_same_account_adds_exactly_the_external_repository" {
  variables {
    backend_external_image = "000000000000.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1"
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 4
    error_message = "With push_external_image on and an in-account, in-region ECR image, the grant must be this project's three prefixes plus exactly one external repository. Got ${jsonencode(output.ecr_repository_arns)}"
  }

  assert {
    condition = alltrue([
      for arn in [
        "arn:aws:ecr:us-east-1:000000000000:repository/acme_backend",
        "arn:aws:ecr:us-east-1:000000000000:repository/acme_service_*",
        "arn:aws:ecr:us-east-1:000000000000:repository/acme_task_*",
      ] : contains(output.ecr_repository_arns, arn)
    ])
    error_message = "Opting in to the external repository must not drop any of this project's three prefixes. Got ${jsonencode(output.ecr_repository_arns)}"
  }

  assert {
    condition = jsonencode(sort(tolist(setsubtract(output.ecr_repository_arns, [
      "arn:aws:ecr:us-east-1:000000000000:repository/acme_backend",
      "arn:aws:ecr:us-east-1:000000000000:repository/acme_service_*",
      "arn:aws:ecr:us-east-1:000000000000:repository/acme_task_*",
      ])))) == jsonencode([
      "arn:aws:ecr:us-east-1:000000000000:repository/shared/acme-api",
    ])
    error_message = "The only ARN added must be the external repository, by its exact name, with the tag stripped. Got ${jsonencode(output.ecr_repository_arns)}"
  }

  # The external repository joins the existing statement; it does not get one
  # of its own. A second statement is where a wildcard resource gets written.
  assert {
    condition = length([
      for s in jsondecode(output.json).Statement : s
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ]) == 1
    error_message = "The external repository must be added to the ONE existing repository-scoped statement, not to a second one. Rendered policy was:\n${output.json}"
  }

  assert {
    condition = jsonencode(sort(output.ecr_repository_arns)) == jsonencode(one([
      for s in jsondecode(output.json).Statement : sort(flatten([s.Resource]))
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ]))
    error_message = "output.ecr_repository_arns must be exactly what the rendered scoped statement grants. Got ${jsonencode(sort(output.ecr_repository_arns))} against:\n${output.json}"
  }

  # (i) Layer download, in the rendered document rather than in a local.
  assert {
    condition = contains(one([
      for s in jsondecode(output.json).Statement : flatten([s.Action])
      if length([for a in flatten([s.Action]) : a if startswith(a, "ecr:") && a != "ecr:GetAuthorizationToken"]) > 0
    ]), "ecr:GetDownloadUrlForLayer")
    error_message = "The repository-scoped statement must grant ecr:GetDownloadUrlForLayer. Without it `docker build --cache-from` and `FROM <ecr image>` fail with AccessDenied after login succeeds. Rendered policy was:\n${output.json}"
  }
}

run "flag_on_external_repository_is_reachable_and_neighbours_are_not" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.flag_on_same_account_adds_exactly_the_external_repository.json

    candidates = {
      external = "arn:aws:ecr:us-east-1:000000000000:repository/shared/acme-api"
    }
  }

  assert {
    condition     = jsonencode(output.ecr_repository_reach["external"]) == jsonencode(sort(run.flag_on_same_account_adds_exactly_the_external_repository.ecr_scoped_actions))
    error_message = "The opted-in external repository must be reachable for the whole push path. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

run "flag_on_does_not_widen_to_any_other_repository" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.flag_on_same_account_adds_exactly_the_external_repository.json

    candidates = {
      circl_backend    = "arn:aws:ecr:us-east-1:000000000000:repository/circl_backend"
      acmecorp_backend = "arn:aws:ecr:us-east-1:000000000000:repository/acmecorp_backend"

      # The external grant is one exact name, not a prefix: a sibling in the
      # same namespace, or a longer name starting with it, stays unreachable.
      shared_sibling = "arn:aws:ecr:us-east-1:000000000000:repository/shared/other-api"
      shared_longer  = "arn:aws:ecr:us-east-1:000000000000:repository/shared/acme-api-v2"

      # The same repository name in another region and another account.
      external_elsewhere_region  = "arn:aws:ecr:eu-west-1:000000000000:repository/shared/acme-api"
      external_elsewhere_account = "arn:aws:ecr:us-east-1:111111111111:repository/shared/acme-api"
    }
  }

  assert {
    condition     = alltrue([for label, actions in output.ecr_repository_reach : length(actions) == 0])
    error_message = "Opting in to one external repository must grant that repository and nothing else. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

# ---------------------------------------------------------------------------
# (b) Flag off: the same image grants nothing.
# ---------------------------------------------------------------------------

run "flag_off_adds_nothing" {
  variables {
    backend_external_image = "000000000000.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1"
    push_external_image    = false
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "Without push_external_image the external image must add no ARN: default false keeps every existing project's grant unchanged. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

run "flag_off_external_repository_is_unreachable" {
  module {
    source = "./tests/matcher"
  }

  variables {
    policy_json = run.flag_off_adds_nothing.json

    candidates = {
      external = "arn:aws:ecr:us-east-1:000000000000:repository/shared/acme-api"
    }
  }

  assert {
    condition     = length(output.ecr_repository_reach["external"]) == 0
    error_message = "Without push_external_image the external repository must be unreachable. Reach was: ${jsonencode(output.ecr_repository_reach)}"
  }
}

# ---------------------------------------------------------------------------
# (c) (d) Flag on, but the image is in another account or another region.
# ---------------------------------------------------------------------------

run "flag_on_other_account_adds_nothing" {
  variables {
    backend_external_image = "111111111111.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1"
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "An image in another account must add no ARN: the identity policy alone cannot grant cross-account push, and the ticket asks for no grant there. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

run "flag_on_other_region_adds_nothing" {
  variables {
    backend_external_image = "000000000000.dkr.ecr.eu-west-1.amazonaws.com/shared/acme-api:v1"
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "An image in another region must add no ARN. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

# ---------------------------------------------------------------------------
# (e) Flag on, but the image is not a private ECR URI at all.
# ---------------------------------------------------------------------------

run "flag_on_docker_hub_image_adds_nothing" {
  variables {
    backend_external_image = "nginx:latest"
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "A Docker Hub image must add no ARN. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

run "flag_on_public_ecr_image_adds_nothing" {
  variables {
    backend_external_image = "public.ecr.aws/nginx/nginx:latest"
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "A public.ecr.aws image must add no ARN. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

run "flag_on_empty_image_adds_nothing" {
  variables {
    backend_external_image = ""
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "An empty backend_external_image must add no ARN, and in particular no ARN with an empty repository field. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

# ---------------------------------------------------------------------------
# (f) A wildcard in the repository name is not parsed, so it cannot reach IAM.
# ---------------------------------------------------------------------------

run "flag_on_wildcard_image_adds_nothing" {
  variables {
    backend_external_image = "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_*:x"
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "An image whose repository name contains `*` must add no ARN: `repository/acme_*` would reach every repository starting with the name, which a YAML value must never be able to grant. Got ${jsonencode(output.ecr_repository_arns)}"
  }

  assert {
    condition     = !anytrue([for arn in output.ecr_repository_arns : strcontains(arn, "acme_*:")])
    error_message = "A wildcard from the image reached the grant. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

# ---------------------------------------------------------------------------
# (g) A digest, with and without a tag, is stripped from the ARN.
# ---------------------------------------------------------------------------

run "flag_on_digest_is_stripped" {
  variables {
    backend_external_image = "000000000000.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    push_external_image    = true
  }

  assert {
    condition     = contains(output.ecr_repository_arns, "arn:aws:ecr:us-east-1:000000000000:repository/shared/acme-api")
    error_message = "A digest-pinned image must grant the bare repository ARN. Got ${jsonencode(output.ecr_repository_arns)}"
  }

  assert {
    condition     = !anytrue([for arn in output.ecr_repository_arns : strcontains(arn, "@") || strcontains(arn, "sha256")])
    error_message = "No granted ARN may carry the image digest; an ARN ending in @sha256:... names no repository and matches nothing. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

run "flag_on_tag_and_digest_are_stripped" {
  variables {
    backend_external_image = "000000000000.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    push_external_image    = true
  }

  assert {
    condition     = contains(output.ecr_repository_arns, "arn:aws:ecr:us-east-1:000000000000:repository/shared/acme-api")
    error_message = "An image with both a tag and a digest must grant the bare repository ARN. Got ${jsonencode(output.ecr_repository_arns)}"
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 4
    error_message = "Tag plus digest must still add exactly one ARN. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}

# ---------------------------------------------------------------------------
# (h) An "external" image that names this project's own backend repository.
# ---------------------------------------------------------------------------

run "flag_on_own_repository_image_adds_no_duplicate" {
  variables {
    backend_external_image = "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_backend:v1"
    push_external_image    = true
  }

  assert {
    condition     = length(output.ecr_repository_arns) == 3
    error_message = "An image naming this project's own backend repository must not duplicate its ARN. Got ${jsonencode(output.ecr_repository_arns)}"
  }
}
