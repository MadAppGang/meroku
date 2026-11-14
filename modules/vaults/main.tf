# Shared SSM vaults. A vault is the SSM path /<env>/<project>/vault/<name>/ and a
# list of secret names declared in the YAML. Each declared name becomes a secret
# of every workload connected to the vault, upper-cased, behind the connection's
# prefix, with the parameter /<env>/<project>/vault/<vault>/<name> as its value.
# See ai_docs/VAULTS.md.
#
# Terraform never creates, reads or deletes a vault parameter's value: values are
# written from the web UI's Vaults tab, and a destroy leaves them in SSM. This
# module grants the roles, hands the secrets back to the workloads and warns at
# plan about a declared secret that has no value yet.

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

locals {
  # Vaults some workload is connected to; only these are granted and checked.
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

# A declared secret with no parameter in SSM is a warning, never an error: the
# plan shows it, and a new task of a connected workload cannot start until the
# value is set. One check per vault in use that declares a secret; a check
# block's own data source cannot use for_each, hence the child module.
module "value_check" {
  source   = "./value_check"
  for_each = { for v in local.vaults_in_use : v => var.vaults[v] if length(var.vaults[v]) > 0 }

  vault = each.key
  path  = local.vault_path[each.key]
  names = each.value
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
# would fail to start until it is set again.
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
  # Every secret each connected workload gets, unsorted. Built from the YAML
  # list alone, so every name is known at plan and nothing waits for SSM.
  workload_secret_entries = { for w, conns in local.workload_connections : w => flatten([
    for c in conns : [
      for n in var.vaults[c.vault] : {
        name      = upper("${c.prefix == "" ? "" : "${c.prefix}_"}${n}")
        valueFrom = "${local.vault_arn[c.vault]}/${n}"
        origin    = "vault \"${c.vault}\" (${local.vault_path[c.vault]}/${n})"
      }
    ]
  ]) }

  # Sorted by name, then parameter, so reordering the YAML changes no task
  # definition. A workload is connected to a vault once, so the parameter makes
  # every key unique and two secrets that map to one name both stay in the list
  # for the collision check to report.
  workload_secrets_sorted = { for w, entries in local.workload_secret_entries : w => [
    for key in sort([for e in entries : "${e.name} ${e.valueFrom}"]) :
    one([for e in entries : e if "${e.name} ${e.valueFrom}" == key])
  ] }

  # The configured name of each workload's read grant. Known at plan; reading it
  # makes the secrets below wait for the grant, so a task definition that lists
  # vault secrets is registered only after its execution role may read them.
  execution_grant_names = { for w, p in aws_iam_role_policy.execution_read : w => p.name }
}
