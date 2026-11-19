# Smoke test: a FIFO subscriber plans under a mock provider. No AWS calls.
# Fixtures are synthetic.
#
# Run: terraform -chdir=modules/event_subscriber test

mock_provider "awscc" {}

run "fifo_subscriber_plans" {
  command = plan

  variables {
    project = "myapp"
    env     = "dev"
    kind    = "fifo"
    bus_arn = "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/0000000000000000000000000"
    subscription = {
      name            = "myapp-events-orders-fifo-dev"
      type            = "FIFO"
      target_arn      = "arn:aws:sqs:ap-southeast-2:000000000000:myapp-orders-dev.fifo"
      dead_letter_arn = "arn:aws:sqs:ap-southeast-2:000000000000:myapp-orders-delivery-dlq-dev.fifo"
      role_arn        = "arn:aws:iam::000000000000:role/meroku-events/myapp/dev/myapp_events_orders_delivery_dev"
      pattern         = "{\"detail-type\":[{\"prefix\":\"ledger.\"}]}"
    }
  }

  assert {
    condition     = length(awscc_eventsv2_subscriber.this) == 1
    error_message = "An enabled subscriber plans one instance."
  }
}

# The size limit is in bytes. 2,500 two-byte characters make filters of about
# 2,600 characters and 5,100 bytes, which AWS refuses.
run "multibyte_filters_over_4096_bytes_are_refused" {
  command = plan

  variables {
    project = "myapp"
    env     = "dev"
    kind    = "standard"
    bus_arn = "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/0000000000000000000000000"
    subscription = {
      name            = "myapp-events-billing-t-dev"
      type            = "UNORDERED"
      target_arn      = "arn:aws:sqs:ap-southeast-2:000000000000:myapp-billing-dev"
      dead_letter_arn = "arn:aws:sqs:ap-southeast-2:000000000000:myapp-billing-delivery-dlq-dev"
      role_arn        = "arn:aws:iam::000000000000:role/meroku-events/myapp/dev/myapp_events_billing_delivery_dev"
      pattern         = "{\"detail-type\":[\"${replace(format("%2500s", ""), " ", "é")}\"]}"
    }
  }

  expect_failures = [awscc_eventsv2_subscriber.this]
}

# 1,900 two-byte characters: under the limit, and each counted as two bytes.
run "multibyte_filters_are_measured_in_bytes" {
  command = plan

  variables {
    project = "myapp"
    env     = "dev"
    kind    = "standard"
    bus_arn = "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/0000000000000000000000000"
    subscription = {
      name            = "myapp-events-billing-t-dev"
      type            = "UNORDERED"
      target_arn      = "arn:aws:sqs:ap-southeast-2:000000000000:myapp-billing-dev"
      dead_letter_arn = "arn:aws:sqs:ap-southeast-2:000000000000:myapp-billing-delivery-dlq-dev"
      role_arn        = "arn:aws:iam::000000000000:role/meroku-events/myapp/dev/myapp_events_billing_delivery_dev"
      pattern         = "{\"detail-type\":[\"${replace(format("%1900s", ""), " ", "é")}\"]}"
    }
  }

  assert {
    condition     = local.filter_configuration_size == length(local.filter_configuration_json) + 1900
    error_message = "The filter size counts each two-byte character as two bytes."
  }
}
