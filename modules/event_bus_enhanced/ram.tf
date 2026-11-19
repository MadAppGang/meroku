# =============================================================================
# Sharing the bus through AWS RAM: one resource share per distinct access
# value, each with the EventsV2 managed permission for that access, and one
# principal association per share entry.
#
# An org/OU-only share keeps allow_external_principals false, so it never
# leaves the organization. A share with an account entry sets it true: an
# account outside the organization gets an invitation it must accept.
# =============================================================================

locals {
  ram_permissions = {
    publish   = "arn:aws:ram::aws:permission/AWSRAMEventBridgeEventBusV2PublishOnly"
    subscribe = "arn:aws:ram::aws:permission/AWSRAMEventBridgeEventBusV2SubscribeOnly"
    full      = "arn:aws:ram::aws:permission/AWSRAMEventBridgeEventBusV2FullAccess"
  }

  share_accesses = var.enabled ? toset([for s in var.share : s.access]) : toset([])

  # "<access>/<kind>/<id>"; app/events_enhanced.go reads these keys back from
  # state, so their shape is a contract.
  share_entries = var.enabled ? { for s in var.share : "${s.access}/${s.kind}/${s.id}" => s } : {}

  needs_organization = anytrue([for s in var.share : contains(["organization", "ou"], s.kind)])
}

data "aws_organizations_organization" "current" {
  count = var.enabled && local.needs_organization ? 1 : 0
}

locals {
  organization = try(data.aws_organizations_organization.current[0], null)

  # The partition comes from the organization's own ARN.
  organization_partition = try(split(":", local.organization.arn)[1], "aws")

  principals = { for k, s in local.share_entries : k => (
    s.kind == "organization" ? local.organization.arn :
    s.kind == "ou" ? "arn:${local.organization_partition}:organizations::${local.organization.master_account_id}:ou/${local.organization.id}/${s.id}" :
    s.id
  ) }
}

resource "aws_ram_resource_share" "bus" {
  for_each = local.share_accesses

  name                      = "${var.project}-${var.env}-events-${each.key}"
  permission_arns           = [local.ram_permissions[each.key]]
  allow_external_principals = anytrue([for s in var.share : s.access == each.key && s.kind == "account"])

  tags = merge(local.tags, { Name = "${var.project}-${var.env}-events-${each.key}" })
}

resource "aws_ram_resource_association" "bus" {
  for_each = local.share_accesses

  resource_share_arn = aws_ram_resource_share.bus[each.key].arn
  resource_arn       = awscc_eventsv2_event_bus.project[0].event_bus_arn
}

resource "aws_ram_principal_association" "bus" {
  for_each = local.share_entries

  resource_share_arn = aws_ram_resource_share.bus[each.value.access].arn
  principal          = local.principals[each.key]

  # Destroy order on teardown: every principal association first, then the
  # guard's destroy revokes what other accounts attached, then the bus. So no
  # other account can attach anything after the revoke.
  depends_on = [terraform_data.bus_guard]
}
