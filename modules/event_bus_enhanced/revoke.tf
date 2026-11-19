# =============================================================================
# Revoking other accounts' subscribers, with scripts/revoke-subscribers.sh.
#
# - change (terraform_data.revoke, create-time): in the apply that changes who
#   may subscribe, revoke the subscribers of accounts no current entry entitles.
# - teardown (terraform_data.bus_guard, destroy-time): when the guard is
#   destroyed (the bus leaves, or terraform destroy), revoke every foreign
#   subscriber and event source, after RAM stopped granting access and before
#   the bus is deleted.
#
# The script needs /bin/sh and AWS CLI v2 on the deploying machine. Its
# credentials are whatever the CLI resolves from the inherited environment, the
# same chain the AWS provider uses; the provider block env/main.hbs generates
# sets neither profile nor assume_role, so no --profile is passed. Secrets other
# terraform children need are removed from its environment.
# =============================================================================

resource "terraform_data" "revoke" {
  count = var.enabled && var.revoke.enabled ? 1 : 0

  # A change of who may subscribe, or of what is owed, replaces the instance,
  # and the create runs the script. lost changes without granted when a
  # principal is associated in RAM that no applied step recorded (an apply
  # interrupted between the two); generate keeps lost as recorded once a run
  # has revoked it, so a settled bus plans no replacement.
  triggers_replace = {
    bus_arn = awscc_eventsv2_event_bus.project[0].event_bus_arn
    granted = join(" ", var.revoke.granted)
    lost    = join(" ", var.revoke.lost)
  }

  # Read back by the next generate: what this step was applied with, and what
  # it still owed. A failed run taints the instance, and the next apply
  # replaces it with the lost list computed from that record.
  input = {
    granted = var.revoke.granted
    lost    = var.revoke.lost
  }

  provisioner "local-exec" {
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh"]
    command     = "${path.module}/scripts/revoke-subscribers.sh"
    environment = {
      EBV2_MODE          = "change"
      EBV2_BUS_ARN       = awscc_eventsv2_event_bus.project[0].event_bus_arn
      EBV2_OWN_ACCOUNT   = data.aws_caller_identity.current.account_id
      EBV2_PROJECT       = var.project
      EBV2_KEEP_ACCOUNTS = join(" ", var.revoke.keep_accounts)
      EBV2_KEEP_ORG      = tostring(var.revoke.keep_org)
      EBV2_KEEP_OUS      = join(" ", var.revoke.keep_ous)
      EBV2_LOST          = join(" ", var.revoke.lost)
      AWS_REGION         = data.aws_region.current.region
      AWS_PAGER          = ""
    }
  }

  # After RAM stops granting access: a removed principal association is
  # destroyed first, because a creator waits for the destroyer of what it
  # depends on.
  depends_on = [aws_ram_principal_association.bus]
}

resource "terraform_data" "bus_guard" {
  count = var.enabled && var.guard ? 1 : 0

  # A new bus (same name, new id) gets a new guard; the old guard's destroy
  # cleans the old bus.
  triggers_replace = awscc_eventsv2_event_bus.project[0].event_bus_arn

  # A destroy-time provisioner may read only self, count.index, each.key and
  # path.*, so everything it needs is here.
  input = {
    bus_arn     = awscc_eventsv2_event_bus.project[0].event_bus_arn
    own_account = data.aws_caller_identity.current.account_id
    project     = var.project
    region      = data.aws_region.current.region
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh"]
    command     = "${path.module}/scripts/revoke-subscribers.sh"
    environment = {
      EBV2_MODE        = "teardown"
      EBV2_BUS_ARN     = self.input.bus_arn
      EBV2_OWN_ACCOUNT = self.input.own_account
      EBV2_PROJECT     = self.input.project
      AWS_REGION       = self.input.region
      AWS_PAGER        = ""
    }
  }

  depends_on = [awscc_eventsv2_event_bus.project]
}
