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
