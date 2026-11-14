# Serverless Valkey for the environment. Access is by security group only: the
# workloads listed in var.access may reach the cache's endpoint, nothing else can,
# and there is no password. See ai_docs/VALKEY.md.

locals {
  ports = [6379, 6380]

  # One entry per distinct security group the listed workloads are seen as, keyed
  # by the plan-known sg_key rather than by workload. Bridge-mode workloads all
  # leave from the EC2 instance security group and share the key "ecs_instance",
  # so they get one rule pair between them; a second identical rule would be
  # refused by AWS as a duplicate. Every other key is "workload:<name>", which
  # never equals "ecs_instance".
  access_groups = {
    for k, ids in { for w in var.access : var.workloads[w].sg_key => var.workloads[w].security_group_id... } :
    k => ids[0]
  }

  ingress = {
    for pair in setproduct(keys(local.access_groups), local.ports) :
    "${pair[0]}/${pair[1]}" => {
      sg_key            = pair[0]
      security_group_id = local.access_groups[pair[0]]
      port              = pair[1]
    }
  }

  tags = {
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_security_group" "valkey" {
  name        = "${var.project}_valkey_${var.env}"
  description = "Serverless Valkey for ${var.project} ${var.env}"
  vpc_id      = var.vpc_id

  # No inline rules: ingress is one standalone rule per security group below, and
  # the cache never opens a connection of its own, so there is no egress.

  tags = merge(local.tags, { Name = "${var.project}_valkey_${var.env}" })
}

# 6379 is the primary endpoint, 6380 the reader endpoint.
resource "aws_vpc_security_group_ingress_rule" "access" {
  for_each = local.ingress

  security_group_id            = aws_security_group.valkey.id
  referenced_security_group_id = each.value.security_group_id
  ip_protocol                  = "tcp"
  from_port                    = each.value.port
  to_port                      = each.value.port
  description                  = "Valkey from ${each.value.sg_key}"

  tags = local.tags

  lifecycle {
    precondition {
      condition     = each.value.security_group_id != null
      error_message = "Valkey access for ${each.value.sg_key}: the workload has no security group. A bridge-mode workload needs an EC2 compute pool."
    }
  }
}

resource "aws_elasticache_serverless_cache" "valkey" {
  name                 = local.cache_name
  description          = "Valkey for ${var.project} ${var.env}"
  engine               = "valkey"
  major_engine_version = var.engine_version

  # Pinned: the endpoint lives in the VPC, reachable only through the security
  # group above.
  connection_type    = "vpc"
  subnet_ids         = var.subnet_ids
  security_group_ids = [aws_security_group.valkey.id]

  # A ceiling only. Setting data_storage.minimum bills that much storage every
  # hour whether it is used or not, so it is never set.
  cache_usage_limits {
    data_storage {
      maximum = var.max_storage_gb
      unit    = "GB"
    }
  }

  tags = merge(local.tags, { Name = local.cache_name })
}
