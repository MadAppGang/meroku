# A stand-in for modules/event_bus_enhanced for
# app/events_subscriber_chain_order_test.go: the same inputs, and the
# dependency edges and lifecycles of the bus, the RAM shares and principal
# associations, the revoke step and the teardown guard (revoke.tf, ram.tf),
# each printing its operations as the subscriber fake does.

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

variable "share" {
  type = list(object({
    kind   = string
    id     = string
    access = string
  }))
  default = []
}

variable "revoke" {
  type = object({
    enabled       = bool
    granted       = optional(list(string), [])
    keep_accounts = optional(list(string), [])
    keep_org      = optional(bool, false)
    keep_ous      = optional(list(string), [])
    lost          = optional(list(string), [])
  })
  default = { enabled = false }
}

variable "guard" {
  type    = bool
  default = false
}

locals {
  share_accesses = var.enabled ? toset([for s in var.share : s.access]) : toset([])
  share_entries  = var.enabled ? { for s in var.share : "${s.access}/${s.kind}/${s.id}" => s } : {}
}

resource "terraform_data" "bus" {
  count = var.enabled ? 1 : 0

  input = "${var.project}-${var.env}"

  lifecycle {
    create_before_destroy = true
  }

  provisioner "local-exec" {
    command = "echo \"start bus-create ${self.id}\" && sleep 1 && echo \"end bus-create ${self.id}\""
  }

  provisioner "local-exec" {
    when    = destroy
    command = "echo \"start bus-delete ${self.id}\" && sleep 1 && echo \"end bus-delete ${self.id}\""
  }
}

resource "terraform_data" "share" {
  for_each = local.share_accesses

  input = each.key
}

resource "terraform_data" "resource_association" {
  for_each = local.share_accesses

  input = {
    share = terraform_data.share[each.key].id
    bus   = terraform_data.bus[0].id
  }
}

resource "terraform_data" "principal" {
  for_each = local.share_entries

  input = {
    key   = each.key
    share = terraform_data.share[each.value.access].id
  }

  provisioner "local-exec" {
    command = "echo \"start principal-create ${self.input.key}\" && sleep 1 && echo \"end principal-create ${self.input.key}\""
  }

  provisioner "local-exec" {
    when    = destroy
    command = "echo \"start principal-delete ${self.input.key}\" && sleep 1 && echo \"end principal-delete ${self.input.key}\""
  }

  depends_on = [terraform_data.bus_guard]
}

resource "terraform_data" "revoke" {
  count = var.enabled && var.revoke.enabled ? 1 : 0

  triggers_replace = {
    bus_arn = terraform_data.bus[0].id
    granted = join(" ", var.revoke.granted)
    lost    = join(" ", var.revoke.lost)
  }

  input = {
    granted = var.revoke.granted
    lost    = var.revoke.lost
  }

  provisioner "local-exec" {
    command = "echo \"start revoke granted=${join(",", self.input.granted)} lost=${join(",", self.input.lost)}\" && sleep 1 && echo \"end revoke\""
  }

  depends_on = [terraform_data.principal]
}

resource "terraform_data" "bus_guard" {
  count = var.enabled && var.guard ? 1 : 0

  triggers_replace = terraform_data.bus[0].id

  input = {
    bus_arn = terraform_data.bus[0].id
    project = var.project
  }

  provisioner "local-exec" {
    when    = destroy
    command = "echo \"start teardown $EBV2_BUS_ARN mode=$EBV2_MODE\" && sleep 1 && echo \"end teardown $EBV2_BUS_ARN\""
    environment = {
      EBV2_MODE    = "teardown"
      EBV2_BUS_ARN = self.input.bus_arn
    }
  }

  depends_on = [terraform_data.bus]
}

output "bus_arn" {
  value = try(terraform_data.bus[0].id, "")
}
