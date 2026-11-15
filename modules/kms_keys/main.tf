# KMS signing keys. Each entry of var.keys is an asymmetric SIGN_VERIFY key with
# an alias; each connected workload's task role may sign, verify or both with the
# keys it is connected to, and its container gets each key's ARN and public key.
# See ai_docs/KMS_KEYS.md.
#
# A retired key is still in var.keys, with no connections: the key stays, and
# nothing grants it or names it to a container. Removing the entry from var.keys
# destroys the key, which AWS turns into a deletion scheduled
# deletion_window_in_days ahead; until then `aws kms cancel-key-deletion` brings
# it back, disabled.

locals {
  tags = {
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
  }
}

# key_spec is ForceNew: AWS cannot change an existing key's type, so a changed
# key_spec replaces the key and every signature made with it stops verifying
# against the new public key. The docs say to add a new key instead.
resource "aws_kms_key" "key" {
  for_each = var.keys

  description              = "${var.project} ${var.env} signing key ${each.key} (meroku)"
  key_usage                = "SIGN_VERIFY"
  customer_master_key_spec = each.value.key_spec
  deletion_window_in_days  = 7
  # AWS rotates symmetric keys only; asymmetric keys cannot be rotated.
  enable_key_rotation = false

  tags = merge(local.tags, { Name = "${var.project}-${var.env}-${each.key}" })
}

resource "aws_kms_alias" "key" {
  for_each = var.keys

  name          = "alias/${var.project}-${var.env}-${each.key}"
  target_key_id = aws_kms_key.key[each.key].key_id
}

data "aws_kms_public_key" "key" {
  for_each = var.keys

  key_id = aws_kms_key.key[each.key].arn
}

locals {
  # Connections grouped by workload, keys sorted by name, so reordering the YAML
  # changes no policy and no task definition. Keys come from the YAML, so they
  # are known at plan.
  connected_workloads = toset([for c in var.connections : c.workload])
  workload_keys = { for w in local.connected_workloads : w => sort([
    for c in var.connections : c.key if c.workload == w
  ]) }
  sign_keys = { for w in local.connected_workloads : w => sort([
    for c in var.connections : c.key if c.workload == w && contains(["sign", "both"], c.access)
  ]) }
  verify_keys = { for w in local.connected_workloads : w => sort([
    for c in var.connections : c.key if c.workload == w && contains(["verify", "both"], c.access)
  ]) }

  workload_policy_statements = { for w in local.connected_workloads : w => concat(
    length(local.sign_keys[w]) > 0 ? [
      {
        Sid      = "KmsSign"
        Effect   = "Allow"
        Action   = ["kms:Sign"]
        Resource = [for k in local.sign_keys[w] : aws_kms_key.key[k].arn]
      },
    ] : [],
    length(local.verify_keys[w]) > 0 ? [
      {
        Sid      = "KmsVerify"
        Effect   = "Allow"
        Action   = ["kms:Verify"]
        Resource = [for k in local.verify_keys[w] : aws_kms_key.key[k].arn]
      },
    ] : [],
    [
      {
        Sid      = "KmsPublicKey"
        Effect   = "Allow"
        Action   = ["kms:GetPublicKey", "kms:DescribeKey"]
        Resource = [for k in local.workload_keys[w] : aws_kms_key.key[k].arn]
      },
    ],
  ) }
}

# One inline policy per connected workload, on its task role: the application
# calls KMS itself. Inline rather than managed for the same reason as
# modules/data_bucket: the backend's task role is at the managed-policy quota.
resource "aws_iam_role_policy" "workload" {
  for_each = local.workload_policy_statements

  name = "${var.project}_kms_keys_${each.key}_${var.env}"
  role = var.workloads[each.key].task_role_name
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = each.value
  })
}
