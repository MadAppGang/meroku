# =============================================================================
# AWS SES Email Domain Configuration
# =============================================================================
# This module sets up SES with proper email deliverability:
# - Domain identity verification
# - DKIM signing (3 CNAME records)
# - SPF authorization (TXT record)
# - Custom MAIL FROM domain (MX + TXT records)
# - DMARC policy (configurable)
# - Extra TXT records (txt_records)
#
# Schema v17: Multi-domain support with optional zone_id per domain
#
# Who owns a TXT record set depends on who created the zone (zone_origin):
#
# - created (default): meroku's configuration is the whole truth. The record
#   sets are aws_route53_record resources with allow_overwrite, so a value not
#   listed is removed on apply.
# - existing: the zone was set up outside meroku and its record sets may hold
#   values somebody else wrote. meroku owns single VALUES, not record sets.
#   Each owned value is a terraform_data instance whose provisioners run
#   scripts/route53-txt.sh: create adds the value to the live set, destroy
#   removes exactly that value, and a set is deleted only when nothing else is
#   left in it. See ai_docs/SES_DNS_OWNERSHIP.md.
# =============================================================================

data "aws_region" "current" {}

# =============================================================================
# Domain Consolidation (Legacy + New Format)
# =============================================================================

locals {
  # Merge legacy single domain with new domains list for backward compatibility
  legacy_domain = var.domain != "" ? [{
    domain              = var.domain
    zone_id             = var.zone_id != "" ? var.zone_id : null
    zone_origin         = var.zone_origin
    enable_mail_from    = var.enable_mail_from ? true : null # Convert to nullable
    mail_from_subdomain = var.mail_from_subdomain != "bounce" ? var.mail_from_subdomain : null
    dmarc_policy        = var.dmarc_policy != "none" ? var.dmarc_policy : null
    dmarc_rua_email     = var.dmarc_rua_email != "" ? var.dmarc_rua_email : null
    spf_record          = null
    spf_ttl             = null
    # A typed null: concat with var.domains fails on a bare null here, because
    # the element types cannot be unified.
    apex_txt_values = tolist(null)
    # An empty list unifies with any list of objects; tolist(null) does not.
    txt_records = []
  }] : []

  all_domains = concat(local.legacy_domain, var.domains)

  # Create map of domains for easy lookup (key = domain name)
  domains_map = { for d in local.all_domains : d.domain => d }

  # Domains whose records go into a Route53 zone, split by who created it.
  created_zone_domains = {
    for k, v in local.domains_map : k => v if v.zone_id != null && coalesce(v.zone_origin, "created") == "created"
  }
  merge_domains = {
    for k, v in local.domains_map : k => v if v.zone_id != null && coalesce(v.zone_origin, "created") == "existing"
  }
}

# =============================================================================
# SES Domain Identity
# =============================================================================

resource "aws_ses_domain_identity" "domain" {
  for_each = local.domains_map

  domain = each.value.domain

  lifecycle {
    precondition {
      condition     = length(local.reserved_txt_names[each.key]) == 0
      error_message = "${each.key}: txt_records may not use ${join(", ", local.reserved_txt_names[each.key])}: meroku writes those records itself (_amazonses for domain verification, _dmarc for the DMARC policy, the MAIL FROM subdomain for bounces). Pick another name."
    }
  }
}

# Domain verification TXT record, in a zone meroku created.
resource "aws_route53_record" "domain_amazonses_verification_record" {
  for_each = local.created_zone_domains

  zone_id         = each.value.zone_id
  name            = "_amazonses.${each.value.domain}"
  type            = "TXT"
  ttl             = "600"
  records         = [aws_ses_domain_identity.domain[each.key].verification_token]
  allow_overwrite = true
}

# =============================================================================
# DKIM Configuration (Email Signing)
# =============================================================================

resource "aws_ses_domain_dkim" "dkim" {
  for_each = local.domains_map

  domain = aws_ses_domain_identity.domain[each.key].domain
}

# DKIM CNAME records (only if zone_id provided). The names hold the SES token,
# so no one else can have a value there, in any zone.
# Creates 3 records per domain using flattened map
resource "aws_route53_record" "domain_amazonses_dkim_record" {
  for_each = merge([
    for domain_key, domain in local.domains_map : {
      for idx in range(3) : "${domain_key}-dkim-${idx}" => {
        domain_key = domain_key
        zone_id    = domain.zone_id
        domain     = domain.domain
        dkim_token = aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[idx]
        idx        = idx
      } if domain.zone_id != null
    }
  ]...)

  zone_id         = each.value.zone_id
  name            = "${each.value.dkim_token}._domainkey.${each.value.domain}"
  type            = "CNAME"
  ttl             = "3600"
  records         = ["${each.value.dkim_token}.dkim.amazonses.com"]
  allow_overwrite = true
}

# =============================================================================
# TXT values: SPF, extra apex values, named records
# =============================================================================

locals {
  default_spf_record = "v=spf1 include:amazonses.com ~all"

  txt_records = { for k, v in local.domains_map : k => v.txt_records != null ? v.txt_records : [] }

  # Route53 holds one string of at most 255 characters per quoted part, so
  # each value is split on the delimiter `" "` and every part longer than 255
  # characters is cut into 255-character pieces joined by that delimiter.
  # Empty parts are dropped. A value that is already split into parts of 255
  # characters or fewer comes out unchanged. One map for every TXT value the
  # module writes, so the rule cannot differ between the apex and a name.
  txt_inputs = distinct(concat(
    [for k, v in local.domains_map : coalesce(v.spf_record, local.default_spf_record)],
    flatten([for k, v in local.domains_map : v.apex_txt_values if v.apex_txt_values != null]),
    flatten([for k, recs in local.txt_records : [for r in recs : r.values]]),
  ))
  txt_split = {
    for x in local.txt_inputs : x => join("\" \"", flatten([for segment in split("\" \"", x) : regexall(".{1,255}", segment)]))
  }

  # The apex TXT record set of each domain: the SPF value first, then any extra
  # values, all at one TTL.
  apex_txt = { for k, v in local.domains_map : k => {
    ttl = coalesce(v.spf_ttl, 600)
    values = [for value in concat(
      [coalesce(v.spf_record, local.default_spf_record)],
      v.apex_txt_values != null ? v.apex_txt_values : [],
    ) : local.txt_split[value]]
  } }

  # Every txt_records entry, keyed "<domain>|<name>".
  txt_record_sets = merge([for k, recs in local.txt_records : {
    for r in recs : "${k}|${r.name}" => {
      domain_key = k
      zone_id    = local.domains_map[k].zone_id
      name       = r.name
      fqdn       = "${r.name}.${lower(local.domains_map[k].domain)}"
      ttl        = r.ttl != null ? r.ttl : 600
      ttl_set    = r.ttl != null
      values     = [for x in r.values : local.txt_split[x]]
    }
  }]...)

  # Names meroku writes itself, refused in txt_records.
  reserved_txt_names = { for k, v in local.domains_map : k => [
    for r in local.txt_records[k] : r.name if contains(concat(
      ["_amazonses", "_dmarc"],
      local.domains_with_mail_from[k].enable_mail_from ? [local.domains_with_mail_from[k].mail_from_subdomain] : [],
    ), r.name)
  ] }
}

# Zones meroku created: Route53 holds one TXT record set per name, and
# allow_overwrite replaces that whole set, so any apex TXT value not listed in
# spf_record or apex_txt_values is removed on apply.
resource "aws_route53_record" "spf" {
  for_each = local.created_zone_domains

  zone_id         = each.value.zone_id
  name            = each.value.domain
  type            = "TXT"
  ttl             = local.apex_txt[each.key].ttl
  records         = local.apex_txt[each.key].values
  allow_overwrite = true
}

# Zones meroku created: each txt_records entry is the whole record set.
resource "aws_route53_record" "txt" {
  for_each = { for k, v in local.txt_record_sets : k => v if contains(keys(local.created_zone_domains), v.domain_key) }

  zone_id         = each.value.zone_id
  name            = each.value.fqdn
  type            = "TXT"
  ttl             = each.value.ttl
  records         = each.value.values
  allow_overwrite = true
}

# =============================================================================
# Custom MAIL FROM Domain (Envelope Sender)
# =============================================================================

# By default, SES uses amazonses.com as the MAIL FROM domain.
# A custom MAIL FROM domain improves deliverability by:
# 1. Ensuring SPF alignment for DMARC
# 2. Making bounce addresses use your domain
# 3. Looking more professional in email headers

locals {
  # Calculate per-domain mail_from settings with proper fallbacks
  domains_with_mail_from = {
    for k, v in local.domains_map : k => {
      domain              = v.domain
      zone_id             = v.zone_id
      enable_mail_from    = coalesce(v.enable_mail_from, var.global_enable_mail_from)
      mail_from_subdomain = coalesce(v.mail_from_subdomain, var.global_mail_from_subdomain)
      mail_from_domain    = "${coalesce(v.mail_from_subdomain, var.global_mail_from_subdomain)}.${v.domain}"
    }
  }

  # Filter to only domains with mail_from enabled
  enabled_mail_from = { for k, v in local.domains_with_mail_from : k => v if v.enable_mail_from }
}

resource "aws_ses_domain_mail_from" "mail_from" {
  for_each = local.enabled_mail_from

  domain           = aws_ses_domain_identity.domain[each.key].domain
  mail_from_domain = each.value.mail_from_domain

  # If MX record lookup fails, use default SES behavior
  behavior_on_mx_failure = "UseDefaultValue"
}

# MX record for custom MAIL FROM domain (only if zone_id provided)
# Points to the regional SES SMTP endpoint. The subdomain exists only for SES
# bounces, in any zone, so these stay whole record sets.
resource "aws_route53_record" "mail_from_mx" {
  for_each = { for k, v in local.enabled_mail_from : k => v if v.zone_id != null }

  zone_id         = each.value.zone_id
  name            = each.value.mail_from_domain
  type            = "MX"
  ttl             = "600"
  records         = ["10 feedback-smtp.${data.aws_region.current.region}.amazonses.com"]
  allow_overwrite = true
}

# SPF record for custom MAIL FROM domain (only if zone_id provided).
# Only SES sends from this subdomain, so it keeps the SES-only value whatever
# the apex spf_record says.
resource "aws_route53_record" "mail_from_spf" {
  for_each = { for k, v in local.enabled_mail_from : k => v if v.zone_id != null }

  zone_id         = each.value.zone_id
  name            = each.value.mail_from_domain
  type            = "TXT"
  ttl             = "600"
  records         = ["v=spf1 include:amazonses.com ~all"]
  allow_overwrite = true
}

# =============================================================================
# DMARC Record (Policy for Failed Authentication)
# =============================================================================

# DMARC tells receiving servers what to do when emails fail SPF/DKIM checks:
# - none: Just report, don't take action (use during initial setup)
# - quarantine: Send to spam folder
# - reject: Block the email entirely

locals {
  domains_with_dmarc = {
    for k, v in local.domains_map : k => {
      domain       = v.domain
      zone_id      = v.zone_id
      dmarc_policy = coalesce(v.dmarc_policy, var.global_dmarc_policy)
      dmarc_rua_email = v.dmarc_rua_email != null ? v.dmarc_rua_email : (
        var.global_dmarc_rua_email != "" ? var.global_dmarc_rua_email : "dmarc-reports@${v.domain}"
      )
    }
  }

  dmarc_value = {
    for k, v in local.domains_with_dmarc : k => "v=DMARC1; p=${v.dmarc_policy}; pct=100; rua=mailto:${v.dmarc_rua_email}"
  }
}

# DMARC in a zone meroku created.
resource "aws_route53_record" "dmarc" {
  for_each = local.created_zone_domains

  zone_id         = each.value.zone_id
  name            = "_dmarc.${each.value.domain}"
  type            = "TXT"
  ttl             = "300"
  records         = [local.dmarc_value[each.key]]
  allow_overwrite = true
}

# =============================================================================
# Zones meroku did not create: one terraform_data instance per owned value
# =============================================================================

locals {
  # A slot is a value of which a name may publish only one: SPF at the apex,
  # DMARC at _dmarc. The slot instance's add removes every other value of its
  # kind in the same write, so the name never publishes two. It is keyed by
  # slot, not by value, so a changed value REPLACES the instance (create
  # before destroy: the new add swaps the value in one write).
  merge_slots = merge(
    { for k, v in local.merge_domains : "${k}|@|spf" => {
      domain_key = k
      zone_id    = v.zone_id
      fqdn       = lower(v.domain)
      slot       = "spf"
      value      = "\"${local.apex_txt[k].values[0]}\""
      ttl        = local.apex_txt[k].ttl
    } },
    { for k, v in local.merge_domains : "${k}|_dmarc|dmarc" => {
      domain_key = k
      zone_id    = v.zone_id
      fqdn       = "_dmarc.${lower(v.domain)}"
      slot       = "dmarc"
      value      = "\"${local.dmarc_value[k]}\""
      ttl        = 300
    } },
  )

  # Every other owned value, keyed by the value's hash: a changed value is a
  # different instance, so the old one is removed and the new one added, and
  # every value beside it stays untouched.
  merge_values = merge(
    merge([for k, v in local.merge_domains : {
      for w in distinct(slice(local.apex_txt[k].values, 1, length(local.apex_txt[k].values))) :
      "${k}|@|${substr(sha256(w), 0, 16)}" => {
        zone_id = v.zone_id
        fqdn    = lower(v.domain)
        value   = "\"${w}\""
        ttl     = local.apex_txt[k].ttl
      }
    }]...),
    merge([for key, set in local.txt_record_sets : {
      for w in distinct(set.values) : "${key}|${substr(sha256(w), 0, 16)}" => {
        zone_id = set.zone_id
        fqdn    = set.fqdn
        value   = "\"${w}\""
        ttl     = set.ttl
      }
    } if contains(keys(local.merge_domains), set.domain_key)]...),
  )

  # The verification token is keyed by purpose: a new token replaces the
  # instance, which removes the old token before adding the new one. Another
  # SES region or account verifying the same domain keeps its own token.
  merge_verification = { for k, v in local.merge_domains : "${k}|_amazonses|verification" => {
    zone_id = v.zone_id
    fqdn    = "_amazonses.${lower(v.domain)}"
    value   = "\"${aws_ses_domain_identity.domain[k].verification_token}\""
    ttl     = 600
  } }

  # A TTL is a property of the record set, not of a value, and a terraform_data
  # input change runs no provisioner. So an explicit TTL is its own instance,
  # replaced when the TTL changes. An unset TTL keeps whatever the set has.
  merge_ttls = merge(
    { for k, v in local.merge_domains : "${k}|@" => {
      zone_id = v.zone_id
      fqdn    = lower(v.domain)
      ttl     = v.spf_ttl
    } if v.spf_ttl != null },
    { for key, set in local.txt_record_sets : key => {
      zone_id = set.zone_id
      fqdn    = set.fqdn
      ttl     = set.ttl
    } if set.ttl_set && contains(keys(local.merge_domains), set.domain_key) },
  )

  route53_txt_script = "${path.module}/scripts/route53-txt.sh"
}

# Live TXT values at the slot names, read at plan for the takeover guards
# below. Only read: nothing here feeds an instance's input, so a second plan
# stays empty whatever the live set holds.
data "aws_route53_records" "live" {
  for_each = merge(
    { for k, v in local.merge_domains : "${k}|@" => { zone_id = v.zone_id, fqdn = lower(v.domain) } },
    { for k, v in local.merge_domains : "${k}|_dmarc" => { zone_id = v.zone_id, fqdn = "_dmarc.${lower(v.domain)}" } },
  )

  zone_id    = each.value.zone_id
  name_regex = "^${replace(each.value.fqdn, ".", "\\.")}\\.$"
}

locals {
  live_txt = { for k, d in data.aws_route53_records.live : k => flatten([
    for rr in d.resource_record_sets : [for r in try(tolist(rr.resource_records), []) : r.value] if rr.type == "TXT"
  ]) }

  live_spf = { for k, v in local.merge_domains : k => [
    for x in local.live_txt["${k}|@"] : x if can(regex("(?i)^v=spf1( |$)", replace(trim(x, "\""), "\" \"", "")))
  ] }
  live_dmarc = { for k, v in local.merge_domains : k => [
    for x in local.live_txt["${k}|_dmarc"] : x if can(regex("(?i)^v=dmarc1 *;", replace(trim(x, "\""), "\" \"", "")))
  ] }

  # meroku replaces a live SPF it did not write only when spf_record says what
  # the domain should publish, so the SES default never silently drops a
  # domain's other senders. Same for DMARC: a customer's p=reject is never
  # lowered to meroku's default p=none unless dmarc_policy or dmarc_rua_email
  # is set for the domain.
  spf_takeover_ok = { for k, v in local.merge_domains : k => v.spf_record != null ? true : alltrue([
    for s in local.live_spf[k] : s == local.merge_slots["${k}|@|spf"].value
  ]) }
  dmarc_takeover_ok = { for k, v in local.merge_domains : k => v.dmarc_policy != null || v.dmarc_rua_email != null ? true : alltrue([
    for s in local.live_dmarc[k] : s == local.merge_slots["${k}|_dmarc|dmarc"].value
  ]) }

  # The live SPF with Amazon SES added before its final `all`, as a value the
  # user can paste.
  live_spf_text = { for k, v in local.merge_domains : k => length(local.live_spf[k]) == 0 ? "" : replace(trim(local.live_spf[k][0], "\""), "\" \"", "") }
  spf_suggestion = { for k, t in local.live_spf_text : k => (
    t == "" ? local.default_spf_record
    : strcontains(t, "include:amazonses.com") ? t
    : can(regex(" [-~?+]?all$", t)) ? replace(t, "/ ([-~?+]?all)$/", " include:amazonses.com $1")
    : "${t} include:amazonses.com"
  ) }

  spf_takeover_message = { for k, v in local.merge_domains : k => join("", [
    "${v.domain} already publishes a sender (SPF) record that meroku did not write: ${join(", ", local.live_spf[k])}. ",
    "A domain can have only one. Set ses.domains[].spf_record to the single record it should publish, ",
    "for example \"${local.spf_suggestion[k]}\", and meroku will replace the current one with it. ",
    "The web app can build this value for you. If meroku wrote this value itself, set spf_record to the value you want.",
  ]) }
  dmarc_takeover_message = { for k, v in local.merge_domains : k => join("", [
    "_dmarc.${v.domain} already publishes a DMARC policy that meroku did not write: ${join(", ", local.live_dmarc[k])}. ",
    "A domain can have only one, and meroku's default (p=${local.domains_with_dmarc[k].dmarc_policy}) may be weaker. ",
    "Set ses.domains[].dmarc_policy (and dmarc_rua_email if reports should go elsewhere) for this domain, ",
    "and meroku will replace the current policy with it.",
  ]) }

  # Live values a slot is about to replace, for the warning below.
  slot_replacements = merge(
    { for k, v in local.merge_domains : "${k} sender (SPF) record" => [
      for s in local.live_spf[k] : s if s != local.merge_slots["${k}|@|spf"].value
    ] if v.spf_record != null },
    { for k, v in local.merge_domains : "_dmarc.${k} DMARC policy" => [
      for s in local.live_dmarc[k] : s if s != local.merge_slots["${k}|_dmarc|dmarc"].value
    ] if v.dmarc_policy != null || v.dmarc_rua_email != null },
  )
}

# A warning, not an error: the plan says which live value a slot replaces.
check "ses_dns_slot_replacements" {
  assert {
    condition = alltrue([for k, s in local.slot_replacements : length(s) == 0])
    error_message = join(" ", [
      for k, s in local.slot_replacements : "${k}: meroku replaces ${join(", ", s)} as configured." if length(s) > 0
    ])
  }
}

# Provisioner environment. Credentials: local-exec inherits the environment of
# the terraform process and the script passes no --profile, so the AWS CLI
# resolves the same credentials the AWS provider does. That holds because the
# provider block env/main.hbs generates sets neither `profile` nor
# `assume_role`; adding either there needs the same here.

resource "terraform_data" "dns_slot" {
  for_each = local.merge_slots

  input = {
    zone_id = each.value.zone_id
    fqdn    = each.value.fqdn
    slot    = each.value.slot
    value   = each.value.value
    ttl     = each.value.ttl
    # A destroy provisioner may read only self, so everything it needs is here.
    script = local.route53_txt_script
    region = data.aws_region.current.region
  }
  triggers_replace = {
    zone_id = each.value.zone_id
    fqdn    = each.value.fqdn
    value   = each.value.value
  }

  lifecycle {
    # The new instance's add already removes every other value of its kind,
    # so the replaced object's destroy (which Terraform skips for create
    # before destroy) has nothing left to do, and there is no moment without
    # the record.
    create_before_destroy = true

    precondition {
      condition     = each.value.slot == "spf" ? local.spf_takeover_ok[each.value.domain_key] : local.dmarc_takeover_ok[each.value.domain_key]
      error_message = each.value.slot == "spf" ? local.spf_takeover_message[each.value.domain_key] : local.dmarc_takeover_message[each.value.domain_key]
    }
  }

  provisioner "local-exec" {
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh", self.input.script]
    command     = "add"
    environment = {
      R53_ZONE_ID = self.input.zone_id
      R53_NAME    = self.input.fqdn
      R53_SLOT    = self.input.slot
      R53_VALUE   = self.input.value
      R53_TTL     = tostring(self.input.ttl)
      AWS_REGION  = self.input.region
      AWS_PAGER   = ""
    }
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh", self.input.script]
    command     = "remove"
    environment = {
      R53_ZONE_ID = self.input.zone_id
      R53_NAME    = self.input.fqdn
      R53_SLOT    = ""
      R53_VALUE   = self.input.value
      R53_TTL     = ""
      AWS_REGION  = self.input.region
      AWS_PAGER   = ""
    }
  }
}

resource "terraform_data" "dns_value" {
  for_each = merge(local.merge_values, local.merge_verification)

  input = {
    zone_id = each.value.zone_id
    fqdn    = each.value.fqdn
    value   = each.value.value
    ttl     = each.value.ttl
    script  = local.route53_txt_script
    region  = data.aws_region.current.region
  }
  triggers_replace = {
    zone_id = each.value.zone_id
    fqdn    = each.value.fqdn
    value   = each.value.value
  }

  provisioner "local-exec" {
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh", self.input.script]
    command     = "add"
    environment = {
      R53_ZONE_ID = self.input.zone_id
      R53_NAME    = self.input.fqdn
      R53_SLOT    = ""
      R53_VALUE   = self.input.value
      R53_TTL     = tostring(self.input.ttl)
      AWS_REGION  = self.input.region
      AWS_PAGER   = ""
    }
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh", self.input.script]
    command     = "remove"
    environment = {
      R53_ZONE_ID = self.input.zone_id
      R53_NAME    = self.input.fqdn
      R53_SLOT    = ""
      R53_VALUE   = self.input.value
      R53_TTL     = ""
      AWS_REGION  = self.input.region
      AWS_PAGER   = ""
    }
  }
}

resource "terraform_data" "dns_ttl" {
  for_each = local.merge_ttls

  input = {
    zone_id = each.value.zone_id
    fqdn    = each.value.fqdn
    ttl     = each.value.ttl
    script  = local.route53_txt_script
    region  = data.aws_region.current.region
  }
  triggers_replace = {
    zone_id = each.value.zone_id
    fqdn    = each.value.fqdn
    ttl     = each.value.ttl
  }

  # After the values, so a set this apply creates gets its TTL too.
  depends_on = [terraform_data.dns_slot, terraform_data.dns_value]

  provisioner "local-exec" {
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh", self.input.script]
    command     = "ttl"
    environment = {
      R53_ZONE_ID = self.input.zone_id
      R53_NAME    = self.input.fqdn
      R53_SLOT    = ""
      R53_VALUE   = ""
      R53_TTL     = tostring(self.input.ttl)
      AWS_REGION  = self.input.region
      AWS_PAGER   = ""
    }
  }
  # No destroy provisioner: leaving the TTL where it is deletes nothing.
}

# =============================================================================
# Test Email Identities (for SES Sandbox mode)
# Account-wide - not per-domain (Schema v18+)
# =============================================================================

resource "aws_ses_email_identity" "emails" {
  count = length(var.test_emails)
  email = var.test_emails[count.index]
}
