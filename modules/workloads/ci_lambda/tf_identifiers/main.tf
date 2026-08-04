# Identifier fan-out for the CI Lambda.
#
# This module has no providers, no resources and no data sources. That is what
# lets a test evaluate it with `terraform init -backend=false && terraform
# apply` in a temporary directory, with no AWS credentials and no network, and
# compare the result against what the Lambda resolves.
#
# It owns the *fan-out and lookup shape* only. Repository names and SSM paths
# arrive as inputs, taken from the resources that really create those objects,
# so this module cannot drift from them.

locals {
  # The only two strings that must agree across the Terraform <-> Go boundary.
  # Go embeds the same file.
  contract = jsondecode(file("${path.module}/../contract/contract.json"))

  backend_id = local.contract.backend_id

  service_ids = { for n in var.service_names : n => n }
  task_ids    = { for n in var.scheduled_task_names : n => "${local.contract.task_id_prefix}${n}" }

  # repository name -> identifier, one row per (repo, id) pair.
  repo_pairs = concat(
    var.backend_repo == "" ? [] : [{ repo = var.backend_repo, id = local.backend_id }],
    [
      for n in var.service_names : {
        repo = lookup(var.service_repos, n, "")
        id   = n
      } if lookup(var.service_repos, n, "") != ""
    ],
    [
      for n in var.scheduled_task_names : {
        repo = lookup(var.task_repos, n, "")
        id   = local.task_ids[n]
      } if lookup(var.task_repos, n, "") != ""
    ],
  )

  repos = distinct([for p in local.repo_pairs : p.repo])

  # SSM path prefix -> identifier. The task prefix (/env/project/task/name) is
  # deliberately longer than a hypothetical service literally named "task"
  # (/env/project/task); the Lambda resolves by longest prefix.
  ssm_pairs = concat(
    var.backend_ssm_prefix == "" ? [] : [{ prefix = var.backend_ssm_prefix, id = local.backend_id }],
    [
      for n in var.service_names : {
        prefix = lookup(var.service_ssm_prefixes, n, "")
        id     = n
      } if lookup(var.service_ssm_prefixes, n, "") != ""
    ],
    [
      for n in var.scheduled_task_names : {
        prefix = lookup(var.task_ssm_prefixes, n, "")
        id     = local.task_ids[n]
      } if lookup(var.task_ssm_prefixes, n, "") != ""
    ],
  )

  # identifier -> may the CI Lambda redeploy this target on its own?
  #
  # This is a *flag*, not a filter. Every target stays in every map: a target
  # removed from ECR_REPO_MAP is indistinguishable from a typo'd repository
  # name, and the Lambda can only answer "no target uses repository X" — which
  # is wrong and sends the reader hunting for a naming bug that does not exist.
  # Carrying the value lets the Lambda say "auto_deploy is disabled for api",
  # which is what actually happened.
  #
  # An absent entry means true. Absent is what a project that predates this
  # setting looks like, and such a project auto-deploys everything today.
  auto_deploy = merge(
    { (local.backend_id) = var.backend_auto_deploy },
    { for n in var.service_names : n => lookup(var.service_auto_deploy, n, true) },
    { for n in var.scheduled_task_names : (local.task_ids[n]) => lookup(var.task_auto_deploy, n, true) },
  )
}
