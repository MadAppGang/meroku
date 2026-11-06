# The plan-time guards of merge mode. In a zone meroku did not create, the
# SPF and DMARC slots may already hold a value someone else wrote. meroku
# replaces it only when the configuration says what the domain should publish;
# otherwise the plan fails and says what to set.
#
# The live record sets come from override_data, so no AWS call is made.
# Fixtures are synthetic. Failing runs come last.
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

run "no_live_values_plans" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing" }]
  }

  assert {
    condition     = length(terraform_data.dns_slot) == 2
    error_message = "With nothing live, the default SPF and DMARC must plan."
  }
}

run "live_value_equal_to_ours_plans" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing" }]
  }

  # What v4.11.1 wrote into this zone: the migration case.
  override_data {
    target = data.aws_route53_records.live["example.com|@"]
    values = {
      resource_record_sets = [{
        name             = "example.com."
        type             = "TXT"
        ttl              = 600
        resource_records = [{ value = "\"v=spf1 include:amazonses.com ~all\"" }, { value = "\"google-site-verification=fake-token-do-not-use\"" }]
      }]
    }
  }
  override_data {
    target = data.aws_route53_records.live["example.com|_dmarc"]
    values = {
      resource_record_sets = [{
        name             = "_dmarc.example.com."
        type             = "TXT"
        ttl              = 300
        resource_records = [{ value = "\"v=DMARC1; p=none; pct=100; rua=mailto:dmarc-reports@example.com\"" }]
      }]
    }
  }

  assert {
    condition     = length(terraform_data.dns_slot) == 2
    error_message = "A live value equal to meroku's must plan: it is the value meroku wrote before."
  }
}

run "explicit_spf_record_replaces_foreign_with_warning" {
  command = plan
  variables {
    domains = [{
      domain      = "example.com"
      zone_id     = "Z000000000000A"
      zone_origin = "existing"
      spf_record  = "v=spf1 include:_spf.google.com include:amazonses.com ~all"
    }]
  }

  override_data {
    target = data.aws_route53_records.live["example.com|@"]
    values = {
      resource_record_sets = [{
        name             = "example.com."
        type             = "TXT"
        ttl              = 3600
        resource_records = [{ value = "\"v=spf1 include:_spf.google.com ~all\"" }]
      }]
    }
  }

  # The check is a warning: the run passes, and the warning is expected.
  expect_failures = [check.ses_dns_slot_replacements]

  assert {
    condition     = terraform_data.dns_slot["example.com|@|spf"].input.value == "\"v=spf1 include:_spf.google.com include:amazonses.com ~all\""
    error_message = "spf_record set explicitly must be planned as the slot value."
  }
}

run "explicit_dmarc_policy_replaces_foreign" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing", dmarc_policy = "reject" }]
  }

  override_data {
    target = data.aws_route53_records.live["example.com|_dmarc"]
    values = {
      resource_record_sets = [{
        name             = "_dmarc.example.com."
        type             = "TXT"
        ttl              = 3600
        resource_records = [{ value = "\"v=DMARC1; p=reject; rua=mailto:sec@example.com\"" }]
      }]
    }
  }

  expect_failures = [check.ses_dns_slot_replacements]

  assert {
    condition     = terraform_data.dns_slot["example.com|_dmarc|dmarc"].input.value == "\"v=DMARC1; p=reject; pct=100; rua=mailto:dmarc-reports@example.com\""
    error_message = "An explicit dmarc_policy must be planned as the slot value."
  }
}

run "other_record_types_and_names_are_ignored" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing" }]
  }

  override_data {
    target = data.aws_route53_records.live["example.com|@"]
    values = {
      resource_record_sets = [
        { name = "example.com.", type = "MX", ttl = 300, resource_records = [{ value = "10 v=spf1.example.com." }] },
        { name = "example.com.", type = "TXT", ttl = 300, resource_records = [{ value = "\"not-spf v=spf1 include:x.example.com ~all\"" }] },
      ]
    }
  }

  assert {
    condition     = length(terraform_data.dns_slot) == 2
    error_message = "Only TXT values that begin with v=spf1 count as SPF."
  }
}

run "refuses_silent_spf_takeover" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing" }]
  }

  override_data {
    target = data.aws_route53_records.live["example.com|@"]
    values = {
      resource_record_sets = [{
        name             = "example.com."
        type             = "TXT"
        ttl              = 3600
        resource_records = [{ value = "\"V=SPF1 include:_spf.google.com\" \" ~all\"" }, { value = "\"google-site-verification=fake-token-do-not-use\"" }]
      }]
    }
  }

  expect_failures = [terraform_data.dns_slot]
}

run "refuses_silent_dmarc_downgrade" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing" }]
  }

  override_data {
    target = data.aws_route53_records.live["example.com|_dmarc"]
    values = {
      resource_record_sets = [{
        name             = "_dmarc.example.com."
        type             = "TXT"
        ttl              = 3600
        resource_records = [{ value = "\"v=DMARC1; p=reject; rua=mailto:sec@example.com\"" }]
      }]
    }
  }

  expect_failures = [terraform_data.dns_slot]
}
