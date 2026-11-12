# One inline policy on the task role of every workload that consumes or produces:
# read access to its own queue, and events:PutEvents on the buses its produces
# entries name. Without an entry a workload cannot publish at all; the owning
# modules grant no PutEvents.
#
# Inline rather than managed: a role holds at most 10 managed policies and the
# backend's task role is already at that quota.
#
# A produces entry with detail_types allows PutEvents only when every event in
# the call has one of them (ForAllValues). ForAllValues is also true for a call
# that carries no such key, so a Null guard requires the key to be present.
# own_source_only does the same for events:source with the workload's own
# EVENT_SOURCE.

locals {
  producers = { for p in var.producers : p.name => p }

  consumer_actions = [
    "sqs:ReceiveMessage",
    "sqs:DeleteMessage",
    "sqs:ChangeMessageVisibility",
    "sqs:GetQueueAttributes",
    "sqs:GetQueueUrl",
  ]

  # Each produces entry's condition keys, before they become a Condition block.
  produce_keys = { for name, p in local.producers : name => [
    for e in p.entries : { for k, v in {
      "events:detail-type" = e.any ? [] : e.detail_types
      "events:source"      = e.own_source_only ? [p.source] : []
    } : k => v if length(v) > 0 }
  ] }

  produce_statements = { for name, p in local.producers : name => [
    for i, e in p.entries : merge(
      {
        Sid      = "PutEvents${i}"
        Effect   = "Allow"
        Action   = "events:PutEvents"
        Resource = local.bus_arns[e.bus]
      },
      length(local.produce_keys[name][i]) > 0 ? {
        Condition = {
          "ForAllValues:StringEquals" = local.produce_keys[name][i]
          Null                        = { for k in keys(local.produce_keys[name][i]) : k => "false" }
        }
      } : {},
    )
  ] }

  consume_statements = { for name in keys(local.consumers) : name => [{
    Sid      = "EventQueue"
    Effect   = "Allow"
    Action   = local.consumer_actions
    Resource = aws_sqs_queue.main[name].arn
  }] }

  policy_workloads = setunion(keys(local.consumers), keys(local.producers))
}

resource "aws_iam_role_policy" "workload" {
  for_each = local.policy_workloads

  name = "${var.project}_events_${each.key}_${var.env}"
  role = var.workloads[each.key].task_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      lookup(local.consume_statements, each.key, []),
      lookup(local.produce_statements, each.key, []),
    )
  })

  lifecycle {
    precondition {
      condition     = !contains(keys(local.producers), each.key) || local.producers[each.key].source == var.workloads[each.key].event_source
      error_message = "The producer ${each.key} was given the source ${try(local.producers[each.key].source, "")}, but its EVENT_SOURCE is ${var.workloads[each.key].event_source}; the two event source formulas have drifted (app/testdata/event_source_vectors.json)."
    }
  }
}
