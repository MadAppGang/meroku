# Which record path a domain takes, by zone_origin.
#
# created (default): aws_route53_record resources hold whole record sets.
# existing: terraform_data instances hold single values (merge mode); the
# record sets that may hold values written by others are not resources.
#
# Every run is a plan against a mock provider: no AWS calls, no provisioner
# runs (provisioners run only on apply). Fixtures are synthetic.
# Refusal runs come last: a run that errors skips every run after it.
#
# Run: timeout 600 terraform -chdir=modules/ses test

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "us-east-1"
    }
  }
  mock_data "aws_route53_records" {
    defaults = {
      resource_record_sets = []
    }
  }
}

variables {
  project                 = "qa"
  env                     = "dev"
  global_enable_mail_from = true
}

run "unset_origin_is_created" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A" }]
  }

  assert {
    condition     = length(aws_route53_record.spf) == 1 && length(aws_route53_record.dmarc) == 1 && length(aws_route53_record.domain_amazonses_verification_record) == 1
    error_message = "A domain without zone_origin must keep the whole-set records of v4.11.1."
  }
  assert {
    condition     = length(terraform_data.dns_slot) == 0 && length(terraform_data.dns_value) == 0 && length(terraform_data.dns_ttl) == 0
    error_message = "A created zone must have no merge-mode instances."
  }
  assert {
    condition     = length(data.aws_route53_records.live) == 0
    error_message = "A created zone must read no live record sets."
  }
}

run "explicit_created_is_the_default" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "created" }]
  }

  assert {
    condition     = length(aws_route53_record.spf) == 1 && length(terraform_data.dns_slot) == 0
    error_message = "zone_origin = created must behave as unset."
  }
}

run "existing_zone_uses_merge_mode" {
  command = plan
  variables {
    domains = [{ domain = "Example.com", zone_id = "Z000000000000A", zone_origin = "existing" }]
  }

  assert {
    condition     = length(aws_route53_record.spf) == 0 && length(aws_route53_record.dmarc) == 0 && length(aws_route53_record.domain_amazonses_verification_record) == 0
    error_message = "An existing zone must not hold the apex, DMARC or verification record sets as resources."
  }
  assert {
    condition     = toset(keys(terraform_data.dns_slot)) == toset(["Example.com|@|spf", "Example.com|_dmarc|dmarc"])
    error_message = "An existing zone must own the SPF and DMARC slots. Got: ${jsonencode(keys(terraform_data.dns_slot))}"
  }
  assert {
    condition     = toset(keys(terraform_data.dns_value)) == toset(["Example.com|_amazonses|verification"])
    error_message = "An existing zone must own its verification token as a value. Got: ${jsonencode(keys(terraform_data.dns_value))}"
  }
  assert {
    condition     = terraform_data.dns_slot["Example.com|@|spf"].input.fqdn == "example.com" && terraform_data.dns_value["Example.com|_amazonses|verification"].input.fqdn == "_amazonses.example.com"
    error_message = "Names given to the script must be lower case."
  }
  assert {
    condition     = length(aws_route53_record.domain_amazonses_dkim_record) == 3 && length(aws_route53_record.mail_from_mx) == 1 && length(aws_route53_record.mail_from_spf) == 1
    error_message = "DKIM CNAMEs and MAIL FROM records stay whole record sets in any zone."
  }
  assert {
    condition     = data.aws_route53_records.live["Example.com|@"].name_regex == "^example\\.com\\.$" && data.aws_route53_records.live["Example.com|_dmarc"].name_regex == "^_dmarc\\.example\\.com\\.$"
    error_message = "The guard must read exactly the apex and _dmarc names."
  }
}

run "mixed_domains_take_their_own_paths" {
  command = plan
  variables {
    domains = [
      { domain = "example.com", zone_id = "Z000000000000A" },
      { domain = "example.org", zone_id = "Z000000000000B", zone_origin = "existing" },
    ]
  }

  assert {
    condition     = keys(aws_route53_record.spf) == ["example.com"] && toset(keys(terraform_data.dns_slot)) == toset(["example.org|@|spf", "example.org|_dmarc|dmarc"])
    error_message = "Each domain must take the path of its own zone."
  }
}

run "legacy_domain_existing" {
  command = plan
  variables {
    domain      = "mail.example.com"
    zone_id     = "Z000000000000C"
    zone_origin = "existing"
    domains     = []
  }

  assert {
    condition     = length(aws_route53_record.spf) == 0 && contains(keys(terraform_data.dns_slot), "mail.example.com|@|spf")
    error_message = "The legacy single domain must honour var.zone_origin."
  }
}

run "legacy_domain_default_created" {
  command = plan
  variables {
    domain  = "mail.example.com"
    zone_id = "Z000000000000C"
    domains = []
  }

  assert {
    condition     = keys(aws_route53_record.spf) == ["mail.example.com"] && length(terraform_data.dns_slot) == 0
    error_message = "The legacy single domain defaults to created."
  }
}

run "existing_without_zone_id_writes_nothing" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_origin = "existing" }]
  }

  assert {
    condition     = length(terraform_data.dns_slot) == 0 && length(terraform_data.dns_value) == 0 && length(aws_route53_record.spf) == 0
    error_message = "Without a zone_id nothing is written to Route53, whatever the origin."
  }
  assert {
    condition     = output.manual_dns_instructions["example.com"].records[1].value == "v=spf1 include:amazonses.com ~all"
    error_message = "Without a zone_id the manual instructions are unchanged."
  }
}

run "refuses_unknown_origin" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "imported" }]
  }
  expect_failures = [var.domains]
}

run "refuses_unknown_legacy_origin" {
  command = plan
  variables {
    domain      = "mail.example.com"
    zone_id     = "Z000000000000C"
    zone_origin = "imported"
  }
  expect_failures = [var.zone_origin]
}
