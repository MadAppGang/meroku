# Shared SSM vaults. A vault is the SSM path /<env>/<project>/vault/<name>/; every
# parameter directly under it becomes a secret of each workload connected to the
# vault, named after its last path segment, upper-cased, behind the connection's
# prefix. See ai_docs/VAULTS.md.
#
# Nothing is created for a vault itself: SSM has no folders. This module reads
# the parameters, grants the roles and hands the secrets back to the workloads.

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

locals {
  # Vaults some workload is connected to; only these are read.
  vaults_in_use = toset([for c in var.connections : c.vault])

  vault_path = { for v in local.vaults_in_use : v => "/${var.env}/${var.project}/vault/${v}" }
  vault_arn = { for v in local.vaults_in_use : v =>
    "arn:${data.aws_partition.current.partition}:ssm:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:parameter${local.vault_path[v]}"
  }

  # Connections grouped by workload. Keys come from the YAML, so they are known
  # at plan.
  connected_workloads = toset([for c in var.connections : c.workload])
  workload_connections = { for w in local.connected_workloads : w => [
    for c in var.connections : c if c.workload == w
  ] }
  read_write_workloads = toset([for c in var.connections : c.workload if c.access == "read_write"])
}

# One read per vault in use.
#
# No depends_on, so the read happens at plan: the secret names are known when
# the task definitions are planned, and the collision checks on them run at
# plan. Adding or removing a parameter in a vault therefore changes every
# connected task definition on the next plan. Not recursive: a vault is flat,
# and a nested path would turn two parameters with the same last segment into
# one secret name twice.
data "aws_ssm_parameters_by_path" "vault" {
  for_each = local.vaults_in_use

  path            = local.vault_path[each.key]
  recursive       = false
  with_decryption = false
}

# Every connected workload's execution role may read its vaults' parameters:
# ECS fetches a container's secrets with the execution role when a task starts.
resource "aws_iam_role_policy" "execution_read" {
  for_each = local.workload_connections

  name = "${var.project}_vaults_read_${each.key}_${var.env}"
  role = var.workloads[each.key].execution_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "VaultSecrets"
        Effect   = "Allow"
        Action   = ["ssm:GetParameters"]
        Resource = sort([for c in each.value : "${local.vault_arn[c.vault]}/*"])
      },
    ]
  })
}

# read_write lets the application itself, on its task role, read and write the
# vault's parameters. There is no delete: a deleted parameter is still listed in
# the running task definitions, and every new task of every connected workload
# would fail to start until the next apply.
resource "aws_iam_role_policy" "task_read_write" {
  for_each = { for w in local.read_write_workloads : w => [
    for c in local.workload_connections[w] : c.vault if c.access == "read_write"
  ] }

  name = "${var.project}_vaults_write_${each.key}_${var.env}"
  role = var.workloads[each.key].task_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "VaultParameters"
        Effect   = "Allow"
        Action   = ["ssm:PutParameter", "ssm:GetParameter", "ssm:GetParameters"]
        Resource = sort([for v in each.value : "${local.vault_arn[v]}/*"])
      },
      {
        # GetParametersByPath is authorised against the path itself.
        Sid      = "VaultPaths"
        Effect   = "Allow"
        Action   = ["ssm:GetParametersByPath"]
        Resource = sort(concat([for v in each.value : local.vault_arn[v]], [for v in each.value : "${local.vault_arn[v]}/*"]))
      },
    ]
  })
}

locals {
  # Every secret each connected workload gets, unsorted.
  workload_secret_entries = { for w, conns in local.workload_connections : w => flatten([
    for c in conns : [
      for p in data.aws_ssm_parameters_by_path.vault[c.vault].names : {
        name      = upper("${c.prefix == "" ? "" : "${c.prefix}_"}${reverse(split("/", p))[0]}")
        valueFrom = p
        origin    = "vault \"${c.vault}\" (${p})"
      }
    ]
  ]) }

  # Sorted by name, then parameter, so reordering the YAML or the SSM listing
  # changes no task definition. A workload is connected to a vault once, so the
  # parameter makes every key unique and two parameters that map to one name
  # both stay in the list for the collision check to report.
  workload_secrets_sorted = { for w, entries in local.workload_secret_entries : w => [
    for key in sort([for e in entries : "${e.name} ${e.valueFrom}"]) :
    one([for e in entries : e if "${e.name} ${e.valueFrom}" == key])
  ] }

  # The configured name of each workload's read grant. Known at plan; reading it
  # makes the secrets below wait for the grant, so a task definition that lists
  # vault secrets is registered only after its execution role may read them.
  execution_grant_names = { for w, p in aws_iam_role_policy.execution_read : w => p.name }
}
