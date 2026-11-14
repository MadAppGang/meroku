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
