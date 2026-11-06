# Apex SPF record value and TTL per entry of var.domains (spf_record, spf_ttl).
#
# Every run is a plan against a mock provider: no AWS calls, no infrastructure.
# Fixtures use example.com and synthetic zone ids only.
# Refusal runs come last: a run that errors skips every run after it.
#
# Run: timeout 300 terraform -chdir=modules/ses test

mock_provider "aws" {}

variables {
  project                 = "qa"
  env                     = "dev"
  global_enable_mail_from = true
}

# REQ-2: unset spf_record and spf_ttl give the default value and TTL 600.
run "defaults_when_both_unset" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A" }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 include:amazonses.com ~all"])
    error_message = "Unset spf_record must give the default SPF value."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 600
    error_message = "Unset spf_ttl must give TTL 600."
  }
}

# REQ-1, REQ-2: spf_record set, spf_ttl unset.
run "override_record_keeps_default_ttl" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = "v=spf1 include:_spf.google.com include:amazonses.com ~all"
    }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 include:_spf.google.com include:amazonses.com ~all"])
    error_message = "spf_record must become the apex SPF value."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 600
    error_message = "spf_ttl unset must stay 600 when only spf_record is set."
  }
}

# REQ-1, REQ-2: spf_ttl set, spf_record unset.
run "override_ttl_keeps_default_record" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_ttl = 300 }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 include:amazonses.com ~all"])
    error_message = "spf_record unset must stay the default when only spf_ttl is set."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 300
    error_message = "spf_ttl must become the record TTL."
  }
}

# REQ-3: spf_ttl = 0 is a value.
run "ttl_zero_is_a_value" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_ttl = 0 }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 0
    error_message = "spf_ttl = 0 must reach the record as 0, not fall back to 600."
  }
}

# REQ-4: characters ~ - = : . reach the record exactly.
run "punctuation_reaches_record_exactly" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = "v=spf1 a:mail.example.com ip4:192.0.2.0/24 -include:x.example.com ~all"
      spf_ttl    = 3600
    }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 a:mail.example.com ip4:192.0.2.0/24 -include:x.example.com ~all"])
    error_message = "The SPF value must reach the record byte for byte."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 3600
    error_message = "spf_ttl must become the record TTL."
  }
}

# REQ-4: an SPF macro written %%{i} in HCL holds %{i}.
run "macro_reaches_record" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = "v=spf1 exists:%%{i}._spf.example.com ~all"
    }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 exists:%%{i}._spf.example.com ~all"])
    error_message = "The macro %%{i} must reach the record unchanged."
  }
  assert {
    condition     = one(aws_route53_record.spf["example.com"].records) == join("", ["v=spf1 exists:%", "{i}._spf.example.com ~all"])
    error_message = "The record must hold a literal %%{i}."
  }
}

# REQ-8: the MAIL FROM SPF record ignores spf_record and spf_ttl.
run "mail_from_spf_unchanged" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = "v=spf1 include:_spf.google.com -all"
      spf_ttl    = 60
    }]
  }

  assert {
    condition     = aws_route53_record.mail_from_spf["example.com"].records == toset(["v=spf1 include:amazonses.com ~all"])
    error_message = "MAIL FROM SPF must stay the SES default whatever spf_record says."
  }
  assert {
    condition     = aws_route53_record.mail_from_spf["example.com"].ttl == 600
    error_message = "MAIL FROM SPF TTL must stay 600 whatever spf_ttl says."
  }
}

# REQ-2: entries are independent of each other.
run "entries_are_independent" {
  command = plan
  variables {
    domains = [
      { domain = "example.com", zone_id = "Z000000000000A", spf_record = "v=spf1 -all", spf_ttl = 120 },
      { domain = "example.org", zone_id = "Z000000000000B" },
    ]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 -all"]) && aws_route53_record.spf["example.com"].ttl == 120
    error_message = "The overriding entry must get its own value and TTL."
  }
  assert {
    condition     = aws_route53_record.spf["example.org"].records == toset(["v=spf1 include:amazonses.com ~all"]) && aws_route53_record.spf["example.org"].ttl == 600
    error_message = "An entry without overrides must keep the defaults."
  }
}

# REQ-9: legacy single-domain form, alone.
run "legacy_domain_alone" {
  command = plan
  variables {
    domain  = "legacy.example.com"
    zone_id = "Z000000000000C"
    domains = []
  }

  assert {
    condition     = aws_route53_record.spf["legacy.example.com"].records == toset(["v=spf1 include:amazonses.com ~all"])
    error_message = "The legacy domain must keep the default SPF value."
  }
  assert {
    condition     = aws_route53_record.spf["legacy.example.com"].ttl == 600
    error_message = "The legacy domain must keep TTL 600."
  }
}

# REQ-9: legacy single-domain form beside an overriding domains entry.
run "legacy_domain_beside_domains" {
  command = plan
  variables {
    domain  = "legacy.example.com"
    zone_id = "Z000000000000C"
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = "v=spf1 include:_spf.google.com -all"
      spf_ttl    = 60
    }]
  }

  assert {
    condition     = aws_route53_record.spf["legacy.example.com"].records == toset(["v=spf1 include:amazonses.com ~all"]) && aws_route53_record.spf["legacy.example.com"].ttl == 600
    error_message = "The legacy domain must keep the defaults beside an overriding entry."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 include:_spf.google.com -all"]) && aws_route53_record.spf["example.com"].ttl == 60
    error_message = "The domains entry must keep its override beside the legacy domain."
  }
}

# REQ-7: no zone_id -> no Route53 record; manual instructions show the override.
run "no_zone_id_manual_instructions" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      spf_record = "v=spf1 include:_spf.google.com ~all"
      spf_ttl    = 60
    }]
  }

  assert {
    condition     = length(aws_route53_record.spf) == 0
    error_message = "A domain without zone_id must create no apex SPF record."
  }
  assert {
    condition     = output.manual_dns_instructions["example.com"].records[1].note == "SPF authorization record"
    error_message = "The second manual record must be the SPF authorization record."
  }
  assert {
    condition     = output.manual_dns_instructions["example.com"].records[1].value == "v=spf1 include:_spf.google.com ~all"
    error_message = "Manual instructions must show the effective SPF value, override included."
  }
  assert {
    condition = one([
      for r in output.manual_dns_instructions["example.com"].records : r.value
      if r.note == "Custom MAIL FROM SPF record"
    ]) == "v=spf1 include:amazonses.com ~all"
    error_message = "Manual MAIL FROM SPF must stay the SES default."
  }
}

# REQ-7: no zone_id and no override -> manual instructions show the default.
run "no_zone_id_default_value" {
  command = plan
  variables {
    domains = [{ domain = "example.com" }]
  }

  assert {
    condition     = output.manual_dns_instructions["example.com"].records[1].value == "v=spf1 include:amazonses.com ~all"
    error_message = "Manual instructions must show the default SPF value when unset."
  }
}

# REQ-6: "v=spf1" followed by end of string is an SPF record and plans.
run "accepts_bare_v_spf1" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "v=spf1" }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1"])
    error_message = "A bare v=spf1 must be accepted and reach the record."
  }
}

# REQ-6 (refusals): last in this file.
run "refuses_empty_spf_record" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "" }]
  }
  expect_failures = [var.domains]
}

run "refuses_spf1x" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "v=spf1x ~all" }]
  }
  expect_failures = [var.domains]
}

run "refuses_uppercase_spf_record" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "V=SPF1 ~all" }]
  }
  expect_failures = [var.domains]
}

run "refuses_non_spf_text" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "google-site-verification=abc" }]
  }
  expect_failures = [var.domains]
}

run "refuses_negative_ttl" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_ttl = -1 }]
  }
  expect_failures = [var.domains]
}

run "refuses_fractional_ttl" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_ttl = 300.5 }]
  }
  expect_failures = [var.domains]
}
