# Connects image-push events between environments that share a registry.
#
# ECR emits a push event only in the registry's own account and region. The
# source environment forwards it to each destination's default event bus, where
# the destination's ordinary ci_ecr_push rule picks it up. The CI Lambda then
# applies auto_deploy exactly as it does for a local push.

locals {
  ecr_this_location = "${data.aws_caller_identity.current.account_id}/${data.aws_region.current.region}"

  # One forwarding rule per account and region: every environment there reads
  # the same default bus. A destination at this account and region is already on
  # the bus the event was emitted to.
  ecr_forward_keys = var.ecr_strategy != "local" ? [] : distinct([
    for a in var.ecr_trusted_accounts : "${a.account_id}/${a.region}"
    if a.forward_image_events && "${a.account_id}/${a.region}" != local.ecr_this_location
  ])

  ecr_forward_destinations = {
    for key in local.ecr_forward_keys : replace(key, "/", "-") => {
      account_id = split("/", key)[0]
      region     = split("/", key)[1]
    }
  }

  ecr_image_events_connected = var.ecr_image_events && var.ecr_strategy == "cross_account"
}

data "aws_iam_policy_document" "ecr_forward_assume" {
  count = length(local.ecr_forward_destinations) > 0 ? 1 : 0

  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ecr_forward" {
  count              = length(local.ecr_forward_destinations) > 0 ? 1 : 0
  name               = module.naming.names["ecr_forward_role"]
  assume_role_policy = data.aws_iam_policy_document.ecr_forward_assume[0].json
}

resource "aws_iam_role_policy" "ecr_forward" {
  count = length(local.ecr_forward_destinations) > 0 ? 1 : 0
  name  = "put-events"
  role  = aws_iam_role.ecr_forward[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "events:PutEvents"
      Resource = [for d in values(local.ecr_forward_destinations) : "arn:aws:events:${d.region}:${d.account_id}:event-bus/default"]
    }]
  })
}

resource "aws_cloudwatch_event_rule" "ecr_forward" {
  for_each = length(local.ci_ecr_repos) > 0 ? local.ecr_forward_destinations : {}

  name          = module.naming.names["ecr_forward_rule_${each.key}"]
  description   = "Forward image pushes to ${each.value.account_id} in ${each.value.region}"
  event_pattern = local.ci_ecr_pattern
}

resource "aws_cloudwatch_event_target" "ecr_forward" {
  for_each = aws_cloudwatch_event_rule.ecr_forward

  rule     = each.value.name
  arn      = "arn:aws:events:${local.ecr_forward_destinations[each.key].region}:${local.ecr_forward_destinations[each.key].account_id}:event-bus/default"
  role_arn = aws_iam_role.ecr_forward[0].arn
}

# A statement rather than aws_cloudwatch_event_bus_policy, which owns the whole
# policy and would remove the statements of other projects sharing this account.
resource "aws_cloudwatch_event_permission" "ecr_source" {
  count = local.ecr_image_events_connected && var.ecr_account_id != "" && var.ecr_account_id != data.aws_caller_identity.current.account_id ? 1 : 0

  principal    = var.ecr_account_id
  statement_id = module.naming.names["ecr_events_permission"]
}
