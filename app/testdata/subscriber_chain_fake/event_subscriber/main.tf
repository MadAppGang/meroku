# A stand-in for modules/event_subscriber with the same inputs and the same
# lifecycle, for app/events_subscriber_chain_order_test.go.
#
# A create or delete prints "start <op> <module>", waits a second, then prints
# "end <op> <module>"; the test reads the lines from `terraform apply -json`
# (provision_progress), where they interleave with every other operation of
# the apply, so it sees any two that overlap.
#
# The pattern is part of input, so a pattern change is an in-place update, as
# on the real subscriber: the same graph node, with no destroy of its own. An
# update runs no provisioner; the test takes its start and end from the
# stream's apply_start and apply_complete.

variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "enabled" {
  type    = bool
  default = true
}

variable "kind" {
  type = string
}

variable "bus_arn" {
  type    = string
  default = ""
}

variable "subscription" {
  type = object({
    name            = string
    type            = string
    target_arn      = string
    dead_letter_arn = string
    role_arn        = string
    pattern         = string
  })
  default = null
}

# The test adds this argument to every rendered subscriber block; the real
# module has no way to know its own address.
variable "log_name" {
  type = string
}

resource "terraform_data" "sub" {
  count = var.enabled ? 1 : 0

  input = {
    log_name = var.log_name
    name     = var.subscription.name
    bus_arn  = var.bus_arn
    target   = var.subscription.target_arn
    pattern  = var.subscription.pattern
  }

  lifecycle {
    create_before_destroy = true
  }

  provisioner "local-exec" {
    command = "echo \"start create ${self.input.log_name}\" && sleep 1 && echo \"end create ${self.input.log_name}\""
  }

  provisioner "local-exec" {
    when    = destroy
    command = "echo \"start delete ${self.input.log_name}\" && sleep 1 && echo \"end delete ${self.input.log_name}\""
  }
}

output "subscriber_arn" {
  value = try(terraform_data.sub[0].id, "")
}
