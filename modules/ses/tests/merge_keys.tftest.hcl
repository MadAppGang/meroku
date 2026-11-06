# Merge mode (zone_origin = "existing"): the exact for_each keys and inputs of
# the terraform_data instances. The keys are the design: a changed slot value
# replaces the slot instance, a changed plain value is a different instance,
# and a TTL is its own instance.
#
# Every run is a plan against a mock provider; provisioners do not run.
# Fixtures are synthetic.
#
# Run: timeout 600 terraform -chdir=modules/ses test

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "eu-west-1"
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

run "slot_inputs" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing" }]
  }

  assert {
    condition = terraform_data.dns_slot["example.com|@|spf"].input == {
      zone_id = "Z000000000000A"
      fqdn    = "example.com"
      slot    = "spf"
      value   = "\"v=spf1 include:amazonses.com ~all\""
      ttl     = 600
      script  = "./scripts/route53-txt.sh"
      region  = "eu-west-1"
    }
    error_message = "The SPF slot input is wrong: ${jsonencode(terraform_data.dns_slot["example.com|@|spf"].input)}"
  }
  assert {
    condition     = terraform_data.dns_slot["example.com|_dmarc|dmarc"].input.value == "\"v=DMARC1; p=none; pct=100; rua=mailto:dmarc-reports@example.com\"" && terraform_data.dns_slot["example.com|_dmarc|dmarc"].input.slot == "dmarc" && terraform_data.dns_slot["example.com|_dmarc|dmarc"].input.ttl == 300
    error_message = "The DMARC slot input is wrong: ${jsonencode(terraform_data.dns_slot["example.com|_dmarc|dmarc"].input)}"
  }
  assert {
    condition     = toset(keys(terraform_data.dns_slot["example.com|@|spf"].triggers_replace)) == toset(["zone_id", "fqdn", "value"])
    error_message = "triggers_replace must hold zone, name and value, never the TTL."
  }
  assert {
    condition     = length(terraform_data.dns_ttl) == 0
    error_message = "An unset spf_ttl must create no TTL instance: the live TTL is kept."
  }
}

run "apex_values_keyed_by_hash" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      zone_origin     = "existing"
      spf_record      = "v=spf1 include:_spf.google.com include:amazonses.com ~all"
      apex_txt_values = ["x-owned=1", "ms=ms00000000", "x-owned=1"]
    }]
  }

  assert {
    condition = toset(keys(terraform_data.dns_value)) == toset([
      "example.com|_amazonses|verification",
      "example.com|@|${substr(sha256("x-owned=1"), 0, 16)}",
      "example.com|@|${substr(sha256("ms=ms00000000"), 0, 16)}",
    ])
    error_message = "Every distinct apex value must be one instance keyed by its hash. Got: ${jsonencode(keys(terraform_data.dns_value))}"
  }
  assert {
    condition     = terraform_data.dns_value["example.com|@|${substr(sha256("x-owned=1"), 0, 16)}"].input.value == "\"x-owned=1\""
    error_message = "A value reaches the script in Route53 form, wrapped in quotes."
  }
  assert {
    condition     = terraform_data.dns_slot["example.com|@|spf"].input.value == "\"v=spf1 include:_spf.google.com include:amazonses.com ~all\""
    error_message = "spf_record must become the SPF slot value."
  }
}

run "changed_apex_value_is_a_new_key" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      zone_origin     = "existing"
      apex_txt_values = ["x-owned=2"]
    }]
  }

  assert {
    condition     = contains(keys(terraform_data.dns_value), "example.com|@|${substr(sha256("x-owned=2"), 0, 16)}") && !contains(keys(terraform_data.dns_value), "example.com|@|${substr(sha256("x-owned=1"), 0, 16)}")
    error_message = "A changed value must be a different instance, not a changed input."
  }
}

run "long_value_is_split_in_route53_form" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      zone_origin     = "existing"
      apex_txt_values = [join("", [for i in range(300) : "a"])]
    }]
  }

  assert {
    condition = one([for k, v in terraform_data.dns_value : v.input.value if startswith(k, "example.com|@|")]) == join("", [
      "\"", join("", [for i in range(255) : "a"]), "\" \"", join("", [for i in range(45) : "a"]), "\"",
    ])
    error_message = "A value over 255 characters must reach the script split, with the delimiter and outer quotes."
  }
}

run "explicit_ttl_is_its_own_instance" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing", spf_ttl = 300 }]
  }

  assert {
    condition     = terraform_data.dns_ttl["example.com|@"].input.ttl == 300 && terraform_data.dns_ttl["example.com|@"].triggers_replace.ttl == 300
    error_message = "spf_ttl must be a TTL instance, replaced when the TTL changes."
  }
  assert {
    condition     = terraform_data.dns_slot["example.com|@|spf"].input.ttl == 300
    error_message = "spf_ttl is also the TTL of a set the slot has to create."
  }
}

run "ttl_zero_is_a_value" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", zone_origin = "existing", spf_ttl = 0 }]
  }

  assert {
    condition     = terraform_data.dns_ttl["example.com|@"].input.ttl == 0
    error_message = "spf_ttl = 0 is a TTL, not unset."
  }
}

run "dmarc_overrides_reach_the_slot" {
  command = plan
  variables {
    global_dmarc_policy = "quarantine"
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      zone_origin     = "existing"
      dmarc_policy    = "reject"
      dmarc_rua_email = "dmarc@example.com"
    }]
  }

  assert {
    condition     = terraform_data.dns_slot["example.com|_dmarc|dmarc"].input.value == "\"v=DMARC1; p=reject; pct=100; rua=mailto:dmarc@example.com\""
    error_message = "The DMARC slot must carry the domain's own policy."
  }
}
