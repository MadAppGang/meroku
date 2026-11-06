# Extra TXT record sets at names below an SES domain (txt_records): the
# 255-character split, the created-zone and existing-zone paths, the manual
# instructions, and the refusals.
#
# Every run is a plan against a mock provider. Fixtures are synthetic: the
# "DKIM key" below is a run of the letters "fake".
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
  fake_key                = "v=DKIM1; k=rsa; p=${join("", [for i in range(100) : "fake"])}"
}

run "absent_creates_nothing" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A" }]
  }

  assert {
    condition     = length(aws_route53_record.txt) == 0
    error_message = "No txt_records, no extra record sets."
  }
}

run "created_zone_whole_set_split" {
  command = plan
  variables {
    domains = [{
      domain      = "example.com"
      zone_id     = "Z000000000000A"
      txt_records = [{ name = "google._domainkey", ttl = 3600, values = [var.fake_key] }]
    }]
  }

  assert {
    condition     = aws_route53_record.txt["example.com|google._domainkey"].name == "google._domainkey.example.com"
    error_message = "The record name is the relative name plus the domain."
  }
  assert {
    condition     = aws_route53_record.txt["example.com|google._domainkey"].ttl == 3600
    error_message = "ttl must reach the record set."
  }
  assert {
    condition = aws_route53_record.txt["example.com|google._domainkey"].records == toset([
      join("\" \"", [substr(var.fake_key, 0, 255), substr(var.fake_key, 255, -1)]),
    ])
    error_message = "A value over 255 characters must be split with the delimiter."
  }
}

run "created_zone_default_ttl_and_several_values" {
  command = plan
  variables {
    domains = [{
      domain      = "example.com"
      zone_id     = "Z000000000000A"
      txt_records = [{ name = "_acme-test.app", values = ["one", "two"] }]
    }]
  }

  assert {
    condition     = aws_route53_record.txt["example.com|_acme-test.app"].ttl == 600 && aws_route53_record.txt["example.com|_acme-test.app"].records == toset(["one", "two"])
    error_message = "Unset ttl gives 600, and every value is in the set."
  }
}

run "existing_zone_values_and_ttl" {
  command = plan
  variables {
    domains = [{
      domain      = "example.com"
      zone_id     = "Z000000000000A"
      zone_origin = "existing"
      spf_record  = "v=spf1 include:_spf.google.com include:amazonses.com ~all"
      txt_records = [
        { name = "google._domainkey", ttl = 3600, values = [var.fake_key] },
        { name = "_acme-test", values = ["one", "two"] },
      ]
    }]
  }

  assert {
    condition     = length(aws_route53_record.txt) == 0
    error_message = "In an existing zone no extra record set is a resource."
  }
  assert {
    condition = terraform_data.dns_value["example.com|google._domainkey|${substr(sha256(join("\" \"", [substr(var.fake_key, 0, 255), substr(var.fake_key, 255, -1)])), 0, 16)}"].input == {
      zone_id = "Z000000000000A"
      fqdn    = "google._domainkey.example.com"
      value   = "\"${join("\" \"", [substr(var.fake_key, 0, 255), substr(var.fake_key, 255, -1)])}\""
      ttl     = 3600
      script  = "./scripts/route53-txt.sh"
      region  = "us-east-1"
    }
    error_message = "The DKIM value must be one owned value, split, in Route53 form."
  }
  assert {
    condition     = length([for k, v in terraform_data.dns_value : k if startswith(k, "example.com|_acme-test|")]) == 2
    error_message = "Every value of a record is its own instance."
  }
  assert {
    condition     = toset(keys(terraform_data.dns_ttl)) == toset(["example.com|google._domainkey"])
    error_message = "Only a record with ttl set gets a TTL instance."
  }
}

run "no_zone_manual_instructions" {
  command = plan
  variables {
    domains = [{
      domain      = "example.com"
      txt_records = [{ name = "google._domainkey", values = ["v=DKIM1; k=rsa; p=fake"] }]
    }]
  }

  assert {
    # Filtered by note: the DKIM entries' names hold tokens unknown at plan.
    condition = [
      for r in output.manual_dns_instructions["example.com"].records : [r.name, r.value] if r.note == "Extra TXT record"
    ] == [["google._domainkey.example.com", "v=DKIM1; k=rsa; p=fake"]]
    error_message = "Manual instructions must list the extra record."
  }
}

run "dns_records_created_lists_extra_records" {
  command = plan
  variables {
    domains = [{
      domain      = "example.com"
      zone_id     = "Z000000000000A"
      txt_records = [{ name = "google._domainkey", values = ["v=DKIM1; k=rsa; p=fake"] }]
    }]
  }

  assert {
    condition     = [for r in output.dns_records_created["example.com"] : r.name if r.note == "Extra TXT record"] == ["google._domainkey.example.com"]
    error_message = "dns_records_created must list the extra record."
  }
}

run "mail_from_subdomain_is_free_when_mail_from_is_off" {
  command = plan
  variables {
    domains = [{
      domain           = "example.com"
      zone_id          = "Z000000000000A"
      enable_mail_from = false
      txt_records      = [{ name = "bounce", values = ["x=1"] }]
    }]
  }

  assert {
    condition     = length(aws_route53_record.txt) == 1
    error_message = "With MAIL FROM off, its subdomain is a name like any other."
  }
}

# --- refusals ----------------------------------------------------------------

run "refuses_upper_case_name" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "Google._domainkey", values = ["x=1"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_trailing_dot" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "google._domainkey.", values = ["x=1"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_apex_name" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "@", values = ["x=1"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_wildcard" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "*.app", values = ["x=1"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_absolute_name" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "google._domainkey.example.com", values = ["x=1"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_duplicate_names" {
  command = plan
  variables {
    domains = [{
      domain      = "example.com"
      zone_id     = "Z000000000000A"
      txt_records = [{ name = "a", values = ["x=1"] }, { name = "a", values = ["x=2"] }]
    }]
  }
  expect_failures = [var.domains]
}

run "refuses_empty_values" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "a", values = [] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_empty_value" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "a", values = ["\" \""] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_spf_value" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "a", values = ["V=SPF1 -all"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_stray_quote" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "a", values = ["say \"hi\""] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_non_printable" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "a", values = ["café"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_fractional_ttl" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "a", ttl = 1.5, values = ["x=1"] }] }]
  }
  expect_failures = [var.domains]
}

run "refuses_amazonses_name" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "_amazonses", values = ["x=1"] }] }]
  }
  expect_failures = [aws_ses_domain_identity.domain]
}

run "refuses_dmarc_name" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", txt_records = [{ name = "_dmarc", values = ["v=DMARC1; p=none"] }] }]
  }
  expect_failures = [aws_ses_domain_identity.domain]
}

run "refuses_mail_from_subdomain" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", mail_from_subdomain = "mail", txt_records = [{ name = "mail", values = ["x=1"] }] }]
  }
  expect_failures = [aws_ses_domain_identity.domain]
}
