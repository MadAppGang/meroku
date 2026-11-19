variable "event_bus_name" {
  description = "The bus the workload's EVENT_BUS_NAME names: the project bus <project>-<env> when it is on, otherwise \"default\". The project bus is also added to the buses the workload may describe."
  type        = string
  default     = "default"
}

# The buses a workload may describe: the default bus, and the project bus when
# event_bus_name names it. Publishing is granted separately, by modules/events,
# for the buses and detail-types a workload's events.produces lists.
locals {
  eventbridge_read_resources = concat(
    ["arn:aws:events:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:event-bus/default"],
    var.event_bus_name == "default" ? [] : ["arn:aws:events:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:event-bus/${var.event_bus_name}"],
    ["arn:aws:events:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:rule/*"],
  )
}

variable "event_bus_arn" {
  description = "The enhanced project bus's ARN when the environment has one, \"\" otherwise. When set, the workload may also describe that bus and this environment's subscribers, and list event buses and subscribers."
  type        = string
  default     = ""
}

# Read access on the enhanced project bus, appended to the read policy after
# its classic statement, which stays as it is. The statements differ in shape
# (one has a Condition), so the conditional chooses between two JSON texts:
# both branches are then strings.
locals {
  eventbridge_v2_read_statements = jsondecode(var.event_bus_arn == "" ? "[]" : jsonencode([
    {
      Sid    = "EventBridgeEnhancedBusRead"
      Effect = "Allow"
      Action = ["events:DescribeEventBus", "events:DescribeSubscriber"]
      Resource = [
        var.event_bus_arn,
        "arn:aws:events:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:subscriber/${var.project}-events-*-${var.env}/*",
      ]
    },
    {
      # modules/naming's short form drops project and env from a subscriber
      # name; the tags keep the grant to this environment.
      Sid      = "EventBridgeEnhancedShortNameSubscriberRead"
      Effect   = "Allow"
      Action   = ["events:DescribeSubscriber"]
      Resource = ["arn:aws:events:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:subscriber/events-*/*"]
      Condition = {
        StringEquals = {
          "aws:ResourceTag/Project"     = var.project
          "aws:ResourceTag/Environment" = var.env
        }
      }
    },
    {
      # Neither action takes a resource.
      Sid      = "EventBridgeEnhancedList"
      Effect   = "Allow"
      Action   = ["events:ListEventBuses", "events:ListSubscribers"]
      Resource = ["*"]
    },
  ]))
}
