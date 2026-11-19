# A stand-in for modules/events for app/events_subscriber_chain_order_test.go.
# It takes the consumers list env/main.hbs rendered and gives the subscribers
# and fifo_subscribers outputs their real keys, with a fake queue per consumer
# that the subscribers depend on through target_arn, as on the real queues.
# A pattern stands for the real one: it changes when the subscription, or the
# set of a consumer's ordered subscriptions, changes.

variable "consumers" {
  type = any
}

locals {
  standard = merge([
    for c in var.consumers : {
      for s in try(c.subscriptions, []) : "${c.name}/${s.key}" => {
        consumer = c.name
        pattern  = jsonencode(s)
      } if s.bus == "project" && !try(s.ordered, false)
    }
  ]...)

  fifo = {
    for c in var.consumers : c.name => jsonencode([
      for s in try(c.subscriptions, []) : s if s.bus == "project" && try(s.ordered, false)
    ]) if length([for s in try(c.subscriptions, []) : s if s.bus == "project" && try(s.ordered, false)]) > 0
  }
}

resource "terraform_data" "queue" {
  for_each = toset([for c in var.consumers : c.name])

  input = each.key
}

output "subscribers" {
  value = {
    for k, s in local.standard : k => {
      name            = "events-${k}"
      type            = "UNORDERED"
      target_arn      = terraform_data.queue[s.consumer].id
      dead_letter_arn = ""
      role_arn        = ""
      pattern         = s.pattern
    }
  }
}

output "fifo_subscribers" {
  value = {
    for c, pattern in local.fifo : c => {
      name            = "events-${c}-fifo"
      type            = "FIFO"
      target_arn      = terraform_data.queue[c].id
      dead_letter_arn = ""
      role_arn        = ""
      pattern         = pattern
    }
  }
}
