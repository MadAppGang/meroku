# Events: the project bus, a queue per consuming workload, the rules that feed
# it, and the dead-letter alarms. See ai_docs/EVENTS.md.
#
# Each consumer has three queues:
#   - the main queue, which the workload reads (EVENT_QUEUE_URL). Its running
#     copies share it, so each event is handled once;
#   - the application DLQ, where SQS moves a message after max_receive_count
#     failed receives;
#   - the delivery DLQ, where EventBridge puts an event it could not deliver to
#     the main queue.
#
# All three use SSE-SQS, the encryption SQS manages itself; no KMS key.
#
# On a classic bus every subscription is a rule (this file). On an enhanced bus
# the project bus is made by modules/event_bus_enhanced and passed in; each
# project-bus subscription is delivered by a subscriber the caller creates from
# the subscribers and fifo_subscribers outputs, through the consumer's delivery
# role (delivery.tf). Ordered subscriptions go to a FIFO queue set (fifo.tf).
# Default-bus subscriptions stay rules on both.

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

locals {
  tags = {
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  consumers = { for c in var.consumers : c.name => c }

  enhanced = var.bus_enabled && var.bus_type == "enhanced"

  # One entry per subscription, keyed "<consumer>/<key>". The key is stable under
  # reordering, so moving an entry in the YAML changes no rule.
  subscriptions = merge([for c in var.consumers : {
    for s in c.subscriptions : "${c.name}/${s.key}" => merge(s, { consumer = c.name })
  }]...)

  # Each subscription's field filters, sorted by path. path is the field split
  # on "." after "detail.", so detail.order.type is ["order", "type"].
  subscription_filters = { for k, s in local.subscriptions : k => [
    for field in sort([for f in s.filters : f.field]) : {
      path   = split(".", trimprefix(field, "detail."))
      values = one([for f in s.filters : f.values if f.field == field])
    }
  ] }

  # shared is how many leading path segments a filter has in common with the
  # one before it in sorted order; depth is that previous filter's path length.
  # Sorting keeps paths with a common parent next to each other.
  subscription_filter_steps = { for k, fs in local.subscription_filters : k => [
    for i, f in fs : merge(f, {
      shared = i == 0 ? 0 : length([
        for j in range(min(length(f.path), length(fs[max(i - 1, 0)].path))) : j
        if alltrue([for q in range(j + 1) : f.path[q] == fs[max(i - 1, 0)].path[q]])
      ])
      depth = i == 0 ? 0 : length(fs[max(i - 1, 0)].path)
    })
  ] }

  # The nested "detail" object of the filters, written as JSON: close the
  # previous filter's objects down to the shared parent, open the new ones,
  # write the value list. Terraform has no deep merge, so the text is built and
  # decoded. "" for a subscription without filters.
  subscription_filter_json = { for k, steps in local.subscription_filter_steps : k => length(steps) == 0 ? "" : join("", concat(
    ["{"],
    flatten([for i, s in steps : [
      i == 0 ? "" : join("", concat([for n in range(s.depth - 1 - s.shared) : "}"], [","])),
      join("", [for seg in slice(s.path, s.shared, length(s.path) - 1) : "${jsonencode(seg)}:{"]),
      "${jsonencode(s.path[length(s.path) - 1])}:${jsonencode(s.values)}",
    ]]),
    [for n in range(length(steps[length(steps) - 1].path) - 1) : "}"],
    ["}"],
  )) }

  # The rule pattern of each subscription: the detail-type (exact, by prefix, or
  # with EventBridge's wildcard operator when it contains *), an optional source,
  # the optional field filters under "detail", and never a replayed event: a
  # replay would deliver every archived event to the queue again. jsonencode
  # sorts keys and a key is added only when set, so a subscription without a
  # wildcard or filters keeps the pattern it always had. jsondecode lets the
  # detail-type shapes share one expression.
  subscription_patterns = { for k, s in local.subscriptions : k => jsonencode(merge(
    {
      "detail-type" = jsondecode(
        s.detail_type == "" ? jsonencode([{ prefix = s.detail_type_prefix }]) :
        strcontains(s.detail_type, "*") ? jsonencode([{ wildcard = s.detail_type }]) :
        jsonencode([s.detail_type])
      )
      "replay-name" = [{ exists = false }]
    },
    s.source != "" ? { source = [s.source] } : {},
    length(s.filters) > 0 ? { detail = jsondecode(local.subscription_filter_json[k]) } : {},
  )) }

  # Delivered by a rule: everything on classic, default-bus entries on enhanced.
  rule_subscriptions = { for k, s in local.subscriptions : k => s if !(local.enhanced && s.bus == "project") }
  # Delivered by one standard enhanced-bus subscriber each.
  subscriber_subscriptions = { for k, s in local.subscriptions : k => s if local.enhanced && s.bus == "project" && !s.ordered }
  # Delivered by the consumer's single FIFO subscriber.
  ordered_subscriptions = { for k, s in local.subscriptions : k => s if local.enhanced && s.bus == "project" && s.ordered }

  # Consumers with a FIFO queue set: those with ordered subscriptions, and
  # those keeping a retired set.
  fifo_consumers     = toset([for c, v in local.consumers : c if v.fifo])
  ordered_consumers  = toset(distinct([for s in values(local.ordered_subscriptions) : s.consumer]))
  rule_consumers     = toset(distinct([for s in values(local.rule_subscriptions) : s.consumer]))
  delivery_consumers = setunion(toset(distinct([for s in values(local.subscriber_subscriptions) : s.consumer])), local.ordered_consumers)

  # The detail-type of a subscriber's pattern, as JSON text. A subscriber has
  # no wildcard operator, so a * at the end is a prefix and a * at the start a
  # suffix; variables.tf refuses every other *.
  enhanced_detail_type_json = { for k, s in merge(local.subscriber_subscriptions, local.ordered_subscriptions) : k => (
    s.detail_type == "" ? jsonencode([{ prefix = s.detail_type_prefix }]) :
    !strcontains(s.detail_type, "*") ? jsonencode([s.detail_type]) :
    endswith(s.detail_type, "*") ? jsonencode([{ prefix = trimsuffix(s.detail_type, "*") }]) :
    jsonencode([{ suffix = trimprefix(s.detail_type, "*") }])
  ) }

  # One pattern branch per subscriber-delivered subscription, as JSON text: the
  # fields of the rule pattern without replay-name, since an enhanced bus has
  # no archive to replay from.
  enhanced_branch_json = { for k, s in merge(local.subscriber_subscriptions, local.ordered_subscriptions) : k => jsonencode(merge(
    { "detail-type" = jsondecode(local.enhanced_detail_type_json[k]) },
    s.source != "" ? { source = [s.source] } : {},
    length(s.filters) > 0 ? { detail = jsondecode(local.subscription_filter_json[k]) } : {},
  )) }

  subscriber_patterns = { for k, s in local.subscriber_subscriptions : k => local.enhanced_branch_json[k] }

  # A FIFO subscriber matches every ordered subscription of its consumer: the
  # one branch, or a root $or of all of them, sorted by key.
  fifo_branch_keys = { for c in local.ordered_consumers : c => [
    for k in sort(keys(local.ordered_subscriptions)) : k if local.ordered_subscriptions[k].consumer == c
  ] }
  fifo_patterns = { for c, ks in local.fifo_branch_keys : c => (
    length(ks) == 1
    ? local.enhanced_branch_json[ks[0]]
    : jsonencode({ "$or" = [for k in ks : jsondecode(local.enhanced_branch_json[k])] })
  ) }

  default_bus_arn = "arn:aws:events:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:event-bus/default"

  bus_names = {
    default = "default"
    project = !var.bus_enabled ? "" : local.enhanced ? "${var.project}-${var.env}" : aws_cloudwatch_event_bus.project[0].name
  }
  bus_arns = {
    default = local.default_bus_arn
    project = !var.bus_enabled ? "" : local.enhanced ? var.enhanced_bus_arn : aws_cloudwatch_event_bus.project[0].arn
  }

  # Fixed queue settings: 4 days of retention on the main queue, 14 on the DLQs,
  # 20-second long polling.
  queue_retention_seconds = 345600
  dlq_retention_seconds   = 1209600
  receive_wait_seconds    = 20
}

resource "aws_cloudwatch_event_bus" "project" {
  count = var.bus_enabled && !local.enhanced ? 1 : 0

  name = "${var.project}-${var.env}"
  tags = merge(local.tags, { Name = "${var.project}-${var.env}" })
}

resource "aws_sqs_queue" "dlq" {
  for_each = local.consumers

  name                      = module.naming.names["dlq/${each.key}"]
  message_retention_seconds = local.dlq_retention_seconds
  sqs_managed_sse_enabled   = true

  tags = merge(local.tags, { Name = module.naming.names["dlq/${each.key}"] })
}

resource "aws_sqs_queue" "delivery_dlq" {
  for_each = local.consumers

  name                      = module.naming.names["delivery_dlq/${each.key}"]
  message_retention_seconds = local.dlq_retention_seconds
  sqs_managed_sse_enabled   = true

  tags = merge(local.tags, { Name = module.naming.names["delivery_dlq/${each.key}"] })
}

resource "aws_sqs_queue" "main" {
  for_each = local.consumers

  name                       = module.naming.names["queue/${each.key}"]
  visibility_timeout_seconds = each.value.visibility_timeout
  message_retention_seconds  = local.queue_retention_seconds
  receive_wait_time_seconds  = local.receive_wait_seconds
  sqs_managed_sse_enabled    = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq[each.key].arn
    maxReceiveCount     = each.value.max_receive_count
  })

  tags = merge(local.tags, { Name = module.naming.names["queue/${each.key}"] })

  lifecycle {
    # Every consumer has a main queue, so this checks an enhanced environment
    # that has no rules too.
    precondition {
      condition     = length(module.naming.collisions) == 0
      error_message = "Two event queues or rules resolved to the same AWS name: ${join(", ", module.naming.collisions)}. Rename a workload or give a subscription a distinct name."
    }
  }
}

resource "aws_cloudwatch_event_rule" "subscription" {
  for_each = local.rule_subscriptions

  name           = module.naming.names["rule/${each.key}"]
  event_bus_name = local.bus_names[each.value.bus]

  event_pattern = local.subscription_patterns[each.key]

  tags = merge(local.tags, { Name = module.naming.names["rule/${each.key}"] })

  lifecycle {
    precondition {
      condition     = length(module.naming.collisions) == 0
      error_message = "Two event queues or rules resolved to the same AWS name: ${join(", ", module.naming.collisions)}. Rename a workload or give a subscription a distinct name."
    }
  }
}

resource "aws_cloudwatch_event_target" "subscription" {
  for_each = local.rule_subscriptions

  rule           = aws_cloudwatch_event_rule.subscription[each.key].name
  event_bus_name = aws_cloudwatch_event_rule.subscription[each.key].event_bus_name
  target_id      = "queue"
  arn            = aws_sqs_queue.main[each.value.consumer].arn

  # 24 hours (the AWS default age) and 185 attempts: an outage shorter than a day
  # delays events instead of moving them to the delivery DLQ. Errors EventBridge
  # cannot retry go to the DLQ at once.
  retry_policy {
    maximum_event_age_in_seconds = 86400
    maximum_retry_attempts       = 185
  }

  dead_letter_config {
    arn = aws_sqs_queue.delivery_dlq[each.value.consumer].arn
  }
}
