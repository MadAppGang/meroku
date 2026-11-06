# Merge mode end to end: real terraform_data provisioners run
# scripts/route53-txt.sh, which calls the fake AWS CLI in
# scripts/test/fakebin. The fake keeps record sets as files, read back here
# with file().
#
# Not part of the default `terraform test` (that reads tests/ only). Run it
# through the wrapper, which puts the fake on PATH, seeds a zone with values
# meroku did not write, and checks after the final destroy that exactly those
# values are left:
#
#   timeout 600 sh modules/ses/tests/lifecycle/run.sh
#
# The AWS provider is mocked; terraform_data is Terraform's own resource, so
# its provisioners really execute. Fixtures are synthetic.

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
  # An apply needs the three tokens the DKIM records index.
  mock_resource "aws_ses_domain_dkim" {
    defaults = {
      dkim_tokens = ["faketoken0", "faketoken1", "faketoken2"]
    }
  }
  mock_resource "aws_ses_domain_identity" {
    defaults = {
      verification_token = "fake-verification-token-do-not-use"
    }
  }
}

variables {
  project                 = "qa"
  env                     = "dev"
  global_enable_mail_from = true

  apex  = "tests/lifecycle/.r53/Z000000000000A/example.com.TXT"
  dmarc = "tests/lifecycle/.r53/Z000000000000A/_dmarc.example.com.TXT"
  acme  = "tests/lifecycle/.r53/Z000000000000A/_acme.example.com.TXT"
  verif = "tests/lifecycle/.r53/Z000000000000A/_amazonses.example.com.TXT"

  spf_google_ses  = "v=spf1 include:_spf.google.com include:amazonses.com ~all"
  spf_with_mailgn = "v=spf1 include:_spf.google.com include:amazonses.com include:mailgun.org ~all"
}

# Seeded by run.sh before this file runs:
#   example.com        TTL 3600  "v=spf1 include:_spf.google.com ~all"
#                                "google-site-verification=fake-token-do-not-use"
#   _dmarc.example.com TTL 3600  "v=DMARC1; p=reject; rua=mailto:sec@example.com"
#   _acme.example.com  TTL 120   "foreign=1"

run "first_apply_into_foreign_sets" {
  command = apply
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      zone_origin     = "existing"
      spf_record      = var.spf_google_ses
      apex_txt_values = ["x-owned=1"]
      dmarc_policy    = "quarantine"
      txt_records     = [{ name = "_acme", values = ["owned=1"] }]
    }]
  }

  assert {
    condition     = split("\n", trimspace(file(var.apex)))[0] == "3600"
    error_message = "The apex set must keep its live TTL. File: ${file(var.apex)}"
  }
  assert {
    condition = toset(slice(split("\n", trimspace(file(var.apex))), 1, length(split("\n", trimspace(file(var.apex)))))) == toset([
      "\"google-site-verification=fake-token-do-not-use\"",
      "\"${var.spf_google_ses}\"",
      "\"x-owned=1\"",
    ])
    error_message = "The apex must hold the foreign verification, meroku's single SPF and meroku's value. File: ${file(var.apex)}"
  }
  assert {
    condition     = split("\n", trimspace(file(var.dmarc))) == tolist(["3600", "\"v=DMARC1; p=quarantine; pct=100; rua=mailto:dmarc-reports@example.com\""])
    error_message = "The DMARC slot must replace the foreign policy, as dmarc_policy says. File: ${file(var.dmarc)}"
  }
  assert {
    condition     = toset(split("\n", trimspace(file(var.acme)))) == toset(["120", "\"foreign=1\"", "\"owned=1\""])
    error_message = "A named record must keep its foreign value and TTL. File: ${file(var.acme)}"
  }
  assert {
    condition     = split("\n", trimspace(file(var.verif))) == tolist(["600", terraform_data.dns_value["example.com|_amazonses|verification"].input.value])
    error_message = "The verification token must be written into a new set with TTL 600. File: ${file(var.verif)}"
  }
}

run "change_the_spf_record" {
  command = apply
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      zone_origin     = "existing"
      spf_record      = var.spf_with_mailgn
      apex_txt_values = ["x-owned=1"]
      dmarc_policy    = "quarantine"
      txt_records     = [{ name = "_acme", values = ["owned=1"] }]
    }]
  }

  assert {
    condition = [
      for v in split("\n", trimspace(file(var.apex))) : v if can(regex("^\"v=spf1", v))
    ] == ["\"${var.spf_with_mailgn}\""]
    error_message = "Exactly one SPF, the new one, must be published. File: ${file(var.apex)}"
  }
}

run "remove_an_owned_value" {
  command = apply
  variables {
    domains = [{
      domain       = "example.com"
      zone_id      = "Z000000000000A"
      zone_origin  = "existing"
      spf_record   = var.spf_with_mailgn
      dmarc_policy = "quarantine"
      txt_records  = [{ name = "_acme", values = ["owned=1"] }]
    }]
  }

  assert {
    condition = toset(split("\n", trimspace(file(var.apex)))) == toset([
      "3600",
      "\"google-site-verification=fake-token-do-not-use\"",
      "\"${var.spf_with_mailgn}\"",
    ])
    error_message = "Only x-owned=1 may be gone. File: ${file(var.apex)}"
  }
}

run "set_the_ttl" {
  command = apply
  variables {
    domains = [{
      domain       = "example.com"
      zone_id      = "Z000000000000A"
      zone_origin  = "existing"
      spf_record   = var.spf_with_mailgn
      spf_ttl      = 300
      dmarc_policy = "quarantine"
      txt_records  = [{ name = "_acme", values = ["owned=1"] }]
    }]
  }

  assert {
    condition     = split("\n", trimspace(file(var.apex)))[0] == "300"
    error_message = "spf_ttl must reach the live set. File: ${file(var.apex)}"
  }
  assert {
    condition     = length(split("\n", trimspace(file(var.apex)))) == 3
    error_message = "Changing the TTL must keep every value. File: ${file(var.apex)}"
  }
}

run "remove_the_named_record" {
  command = apply
  variables {
    domains = [{
      domain       = "example.com"
      zone_id      = "Z000000000000A"
      zone_origin  = "existing"
      spf_record   = var.spf_with_mailgn
      spf_ttl      = 300
      dmarc_policy = "quarantine"
    }]
  }

  assert {
    condition     = split("\n", trimspace(file(var.acme))) == tolist(["120", "\"foreign=1\""])
    error_message = "Removing a txt_records entry removes only meroku's value. File: ${file(var.acme)}"
  }
}

# The framework destroys everything after the last run; run.sh then checks
# the zone holds only what was foreign and was not replaced.
