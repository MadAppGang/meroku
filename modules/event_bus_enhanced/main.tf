# =============================================================================
# The project's enhanced EventBridge bus (EventsV2), <project>-<env>.
#
# ram.tf shares it with other accounts, an OU or the organization. revoke.tf
# revokes other accounts' subscribers when they lose access, and every foreign
# subscriber and event source before the bus is deleted. Subscribers live in
# modules/event_subscriber; queues and delivery roles in modules/events.
# =============================================================================

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

locals {
  bus_name = "${var.project}-${var.env}"

  tags = {
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "awscc_eventsv2_event_bus" "project" {
  count = var.enabled ? 1 : 0

  name        = local.bus_name
  description = "Project bus of ${local.bus_name}, managed by meroku"

  # A delivery-DLQ record arrives at most 24 hours after publish and is kept
  # for 14 days. Replaying it needs the event still on the bus: 24 hours plus
  # 14 days, plus one day of margin.
  storage_configuration = {
    retention_period_in_days = 16
  }

  # awscc has no default_tags.
  tags = [for k, v in local.tags : { key = k, value = v }]

  lifecycle {
    # The subscribers force this onto their dependencies anyway. The name never
    # changes, so the bus is never replaced.
    create_before_destroy = true
  }
}
