# Add EFS ingress rule to backend security group
resource "aws_security_group_rule" "backend_efs_access" {
  for_each = { for mount in var.backend_efs_mounts : mount.efs_name => var.available_efs[mount.efs_name].security_group }

  type                     = "egress"
  from_port                = 2049
  to_port                  = 2049
  protocol                 = "tcp"
  source_security_group_id = each.value
  security_group_id        = aws_security_group.backend.id
  description             = "Allow EFS mount access for ${each.key}"
}



# Add IAM permissions to task role
data "aws_iam_policy_document" "efs_access" {
  statement {
    effect = "Allow"
    actions = [
      "elasticfilesystem:ClientMount",
      "elasticfilesystem:ClientWrite",
      "elasticfilesystem:DescribeMountTargets",
      "elasticfilesystem:ClientRootAccess"
    ]
    resources = [
      for mount in var.backend_efs_mounts :
      "arn:aws:elasticfilesystem:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:file-system/${var.available_efs[mount.efs_name].id}"
    ]
  }
}

resource "aws_iam_role_policy" "efs_access" {
  name   = "efs-access"
  role   = aws_iam_role.backend_task.id
  policy = data.aws_iam_policy_document.efs_access.json
}