resource "aws_efs_file_system" "this" {
  for_each = { for efs in var.efs_configs : efs.name => efs }

  creation_token = "${var.project}-${each.key}-${var.env}"
  encrypted      = true

  tags = {
    Name      = "${var.project}-${each.key}-${var.env}"
    terraform = "true"
    env       = var.env
  }
}

resource "aws_security_group" "efs" {
  for_each = aws_efs_file_system.this

  name   = "${var.project}-efs-${each.key}-${var.env}"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 2049
    to_port     = 2049
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_efs_mount_target" "this" {
  for_each = {
    for pair in setproduct(keys(aws_efs_file_system.this), var.private_subnets) : 
    "${pair[0]}-${pair[1]}" => {
      efs_name = pair[0]
      subnet_id = pair[1]
    }
  }

  file_system_id  = aws_efs_file_system.this[each.value.efs_name].id
  subnet_id       = each.value.subnet_id
  security_groups = [aws_security_group.efs[each.value.efs_name].id]
}

resource "aws_efs_access_point" "this" {
  for_each = { for efs in var.efs_configs : efs.name => efs }

  file_system_id = aws_efs_file_system.this[each.key].id

  posix_user {
    gid = 0
    uid = 0
  }
  root_directory {
    path = each.value.path
    creation_info {
      owner_gid   = each.value.gid
      owner_uid   = each.value.uid
      permissions = each.value.permissions
    }
  }
}
