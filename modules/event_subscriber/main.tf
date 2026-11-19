# =============================================================================
# One subscriber of the enhanced project bus, delivering to a consumer's queue.
#
# Every subscriber gets two filters, which an event must both match: the
# subscription's pattern on the event (DATA), and a SYSTEM_METADATA filter that
# admits only PutEvents entries, so a PutRawEvents payload never reaches a
# consumer queue. A JSONata transformer writes the classic EventBridge
# envelope, with the PutEvents EventId as its id, so a consumer reads the same
# body from either bus kind.
#
# env/main.hbs chains the subscriber modules with depends_on, so the bus sees
# one subscriber create or delete at a time, and every new subscriber exists
# before an old one is deleted.
# =============================================================================

locals {
  tags = {
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  # The id PutEvents returns, which a delivery-DLQ record also names. Neither
  # the event's Data.id nor a RAW delivery carries it.
  event_id_expression = "{% $events.SystemMetadata.\"aws:EventId\" %}"

  # FIFO group: the publishing account and the producer's EventGroupId, so the
  # same group id from two accounts stays two groups. Without an EventGroupId
  # AWS fills in a random one, so such an event is a group of its own; the
  # event-id branch only guards an empty value.
  message_group_expression = "{% $exists($events.SystemMetadata.EventGroupId) and $events.SystemMetadata.EventGroupId != \"\" ? $events.Data.account & \":\" & $events.SystemMetadata.EventGroupId : $events.SystemMetadata.\"aws:EventId\" %}"

  # Only PutEvents entries carry this content type.
  put_events_only_pattern = jsonencode({ ContentType = ["application/eventbridge+json"] })

  # The classic envelope: the same nine keys in the same order.
  envelope_expression = "{% {\"version\": \"0\", \"id\": $events.SystemMetadata.\"aws:EventId\", \"detail-type\": $events.Data.`detail-type`, \"source\": $events.Data.source, \"account\": $events.Data.account, \"time\": $events.Data.time, \"region\": $events.Data.region, \"resources\": $events.Data.resources, \"detail\": $events.Data.detail} %}"

  # AWS limits the JSON text of the whole FilterConfiguration, each pattern
  # escaped as a JSON string inside it, to 4096 bytes. jsonencode also escapes
  # <, > and &, so this is never more lenient than AWS.
  filter_configuration_size = var.subscription == null ? 0 : length(jsonencode({
    Filters = [
      { Pattern = var.subscription.pattern, Scope = "DATA" },
      { Pattern = local.put_events_only_pattern, Scope = "SYSTEM_METADATA" },
    ]
  }))
}

resource "awscc_eventsv2_subscriber" "this" {
  count = var.enabled ? 1 : 0

  # Name, bus, target, type and starting position are create-only. Each is
  # fixed by the module address, so a YAML change never replaces in place.
  name              = var.subscription.name
  event_bus_arn     = var.bus_arn
  type              = var.subscription.type
  starting_position = "LATEST"

  invoke_configuration = {
    target_arn = var.subscription.target_arn
    role_arn   = var.subscription.role_arn
    sqs_parameters = var.kind == "fifo" ? {
      message_group_id         = local.message_group_expression
      message_deduplication_id = local.event_id_expression
    } : null
  }

  filter_configuration = {
    filters = [
      { scope = "DATA", pattern = var.subscription.pattern },
      { scope = "SYSTEM_METADATA", pattern = local.put_events_only_pattern },
    ]
  }

  transformer = {
    type = "JSONATA"
    jsonata_configuration = {
      expression = local.envelope_expression
    }
  }

  # At most 24 hours, the longest AWS allows; the delivery DLQ gets the record
  # after that. The whole object is replaced on update, so every member is set.
  retry_policy = {
    max_event_age_in_seconds = 86400
    max_retry_attempts       = 185
    retry_strategy           = "ALL"
  }

  on_failure_configuration = {
    arn = var.subscription.dead_letter_arn
  }

  # awscc has no default_tags.
  tags = [for k, v in local.tags : { key = k, value = v }]

  lifecycle {
    # A replacement creates the new subscriber before deleting the old one. It
    # also makes a tombstone's delete wait for every create and update
    # downstream of it in the chain.
    create_before_destroy = true

    precondition {
      condition     = (var.kind == "fifo") == (var.subscription.type == "FIFO")
      error_message = "The subscriber ${var.subscription.name} is rendered as ${var.kind} but modules/events made it ${var.subscription.type}; app/events.go and modules/events disagree."
    }
    precondition {
      condition     = local.filter_configuration_size <= 4096
      error_message = "The event filters of ${var.subscription.name} are ${local.filter_configuration_size} bytes as AWS counts them (the JSON of both filters, patterns escaped); the limit is 4096."
    }
    precondition {
      # Form 2 of modules/naming starts with the project; form 3 with "events-".
      condition     = startswith(var.subscription.name, "${var.project}-events-") || startswith(var.subscription.name, "events-")
      error_message = "The subscriber name ${var.subscription.name} is not a modules/naming subscriber name."
    }
  }
}
