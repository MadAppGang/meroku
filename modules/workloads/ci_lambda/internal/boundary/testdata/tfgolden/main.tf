# Synthetic project used to capture what Terraform emits to the CI Lambda.
#
# `go test -tags tfgolden ./internal/boundary/ -update` runs this module and
# rewrites ../tf_identifiers.golden.json. The always-on boundary test then
# feeds that file to the real config loader and resolvers, so both sides of the
# Terraform <-> Go boundary are checked on every `go test`, with no terraform
# binary required.
#
# The project deliberately contains every awkward case:
#   - a hyphenated service name, which the old \w+ regex could never match;
#   - two services sharing one ECR repository (ecr_config mode = use_existing);
#   - a service literally named "task", so longest-prefix SSM matching is
#     actually exercised against the scheduled-task prefix;
#   - a scheduled task, whose SSM path has one segment more than a service's;
#   - a service with auto_deploy = false, which must stay in every map and be
#     refused by name rather than vanish;
#   - a scheduled task with auto_deploy = false, likewise;
#   - "legacy-api", an off-prefix repository (ecr_config mode = manual_repo),
#     which the ECR rule's >2,048-character prefix fallback cannot cover and so
#     has to be listed explicitly.
#
# All values are synthetic. No real account IDs, ARNs or project names.

locals {
  # Declared once and both passed to the module and captured in the output, so
  # the boundary test derives its expectations from the same values Terraform
  # was given rather than from a restatement of them.
  service_auto_deploy = {
    api            = true
    payment-worker = false # disabled: present everywhere, deployed by nothing
    reporting      = true
    task           = true
    legacy-api     = true
  }

  task_auto_deploy = {
    cleanup = true
    archive = false # disabled
  }

  backend_auto_deploy = true
}

module "ids" {
  source = "../../../../tf_identifiers"

  project              = "acme"
  env                  = "dev"
  service_names        = ["api", "legacy-api", "payment-worker", "reporting", "task"]
  scheduled_task_names = ["archive", "cleanup"]

  backend_repo = "acme_backend"
  service_repos = {
    api            = "acme_service_api"
    legacy-api     = "team/legacy-api" # manual_repo: carries no project prefix
    payment-worker = "acme_service_payment-worker"
    reporting      = "acme_service_api" # use_existing: shares api's repository
    task           = "acme_service_task"
  }
  task_repos = {
    archive = "acme_task_archive"
    cleanup = "acme_task_cleanup"
  }

  backend_ssm_prefix = "/dev/acme/backend"
  service_ssm_prefixes = {
    api            = "/dev/acme/api"
    legacy-api     = "/dev/acme/legacy-api"
    payment-worker = "/dev/acme/payment-worker"
    reporting      = "/dev/acme/reporting"
    task           = "/dev/acme/task"
  }
  task_ssm_prefixes = {
    archive = "/dev/acme/task/archive"
    cleanup = "/dev/acme/task/cleanup"
  }

  backend_auto_deploy = local.backend_auto_deploy
  service_auto_deploy = local.service_auto_deploy
  task_auto_deploy    = local.task_auto_deploy
}

locals {
  # These mirror the jsonencode expressions in modules/workloads/lambda.tf.
  # Only the identifier *keys* matter here; the resource names are stand-ins.
  ecs_service_map = jsonencode(merge(
    {
      (module.ids.backend_id) = {
        service_name = "acme_service_dev"
        task_family  = "acme_service_dev"
      }
    },
    {
      for name, id in module.ids.service_ids : (id) => {
        service_name = "acme_service_${name}_dev"
        task_family  = "acme_service_${name}_dev"
      }
    }
  ))

  scheduled_task_map = jsonencode({
    for name, id in module.ids.task_ids : (id) => {
      task_family = "acme_task_${name}_dev"
      type        = "scheduled_task"
    }
  })

  auto_deploy_map = jsonencode(module.ids.auto_deploy)

  # Backend files and per-service files, bucket names verbatim. A disabled
  # service keeps its entry: shared.env below is consumed by one enabled and one
  # disabled service, which is what exercises a partial fan-out.
  s3_service_map = jsonencode({
    (module.ids.backend_id)                    = [{ bucket = "acme-config", key = "backend.env" }]
    (module.ids.service_ids["api"])            = [{ bucket = "acme-config", key = "api.env" }]
    (module.ids.service_ids["payment-worker"]) = [{ bucket = "acme-config", key = "shared.env" }]
    (module.ids.service_ids["reporting"])      = [{ bucket = "acme-config", key = "shared.env" }]
  })
}

output "golden" {
  value = {
    identifiers = {
      backend_id     = module.ids.backend_id
      service_ids    = module.ids.service_ids
      task_ids       = module.ids.task_ids
      ecr_repo_ids   = module.ids.ecr_repo_ids
      ssm_prefix_ids = module.ids.ssm_prefix_ids
      auto_deploy    = module.ids.auto_deploy
    }
    inputs = {
      project             = "acme"
      backend_auto_deploy = local.backend_auto_deploy
      service_auto_deploy = local.service_auto_deploy
      task_auto_deploy    = local.task_auto_deploy
    }
    env = {
      PROJECT_NAME       = "acme"
      PROJECT_ENV        = "dev"
      ECS_CLUSTER_NAME   = "acme_cluster_dev"
      ECS_SERVICE_MAP    = local.ecs_service_map
      SCHEDULED_TASK_MAP = local.scheduled_task_map
      S3_SERVICE_MAP     = local.s3_service_map
      ECR_REPO_MAP       = jsonencode(module.ids.ecr_repo_ids)
      SSM_SERVICE_MAP    = jsonencode(module.ids.ssm_prefix_ids)
      AUTO_DEPLOY_MAP    = local.auto_deploy_map
    }
  }
}
