# Extra apex TXT values (apex_txt_values), automatic 255-character splitting of
# every apex value, manual_dns_instructions for domains without zone_id, and
# the refusals on var.domains.
#
# In HCL the delimiter `" "` is written "\" \"". Long strings are built with
# join("", [for i in range(N) : "x"]) so their length is exact.
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

# REQ-11: unset apex_txt_values keeps the single SPF value.
run "unset_list_single_value" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A" }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 include:amazonses.com ~all"])
    error_message = "Unset apex_txt_values must leave the SPF value alone in the record set."
  }
}

# REQ-11: an empty list is the same as unset.
run "empty_list_single_value" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = [] }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 include:amazonses.com ~all"])
    error_message = "An empty apex_txt_values must leave the SPF value alone in the record set."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 600
    error_message = "An empty apex_txt_values must keep TTL 600."
  }
}

# REQ-11: extra values join the SPF value in the same record set, same TTL.
run "extra_values_in_same_record_set" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      spf_record      = "v=spf1 include:_spf.google.com include:amazonses.com ~all"
      spf_ttl         = 300
      apex_txt_values = ["google-site-verification=fake-token-do-not-use", "ms=ms00000000"]
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      "v=spf1 include:_spf.google.com include:amazonses.com ~all",
      "google-site-verification=fake-token-do-not-use",
      "ms=ms00000000",
    ])
    error_message = "The record set must hold the SPF value plus every extra value."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 300
    error_message = "Extra values share the spf_ttl of the record set."
  }
}

# REQ-11: extras beside the default SPF value.
run "extra_values_with_default_spf" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["google-site-verification=abc"] }]
  }

  assert {
    condition     = aws_route53_record.spf["example.com"].records == toset(["v=spf1 include:amazonses.com ~all", "google-site-verification=abc"])
    error_message = "Extras must sit beside the default SPF value."
  }
  assert {
    condition     = aws_route53_record.spf["example.com"].ttl == 600
    error_message = "TTL stays 600 when spf_ttl is unset."
  }
}

# REQ-12: 255 characters or fewer are unchanged.
run "split_255_unchanged" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      spf_record      = join("", ["v=spf1 ", join("", [for i in range(248) : "x"])])
      apex_txt_values = [join("", [for i in range(255) : "y"])]
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      join("", ["v=spf1 ", join("", [for i in range(248) : "x"])]),
      join("", [for i in range(255) : "y"]),
    ])
    error_message = "Values of exactly 255 characters must be unchanged."
  }
}

# REQ-12: 256 characters -> 255, delimiter, 1.
run "split_256_spf" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = join("", ["v=spf1 ", join("", [for i in range(249) : "x"])])
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      join("", ["v=spf1 ", join("", [for i in range(248) : "x"]), "\" \"", "x"]),
    ])
    error_message = "A 256-character SPF value must split into 255 + delimiter + 1."
  }
}

# REQ-12: 511 characters -> 255, delimiter, 255, delimiter, 1 (extra value).
run "split_511_extra" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      apex_txt_values = [join("", [for i in range(511) : "y"])]
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      "v=spf1 include:amazonses.com ~all",
      join("\" \"", [join("", [for i in range(255) : "y"]), join("", [for i in range(255) : "y"]), "y"]),
    ])
    error_message = "A 511-character extra value must split into 255 + 255 + 1."
  }
}

# Contract: length never refuses; a 600-character spf_record plans and splits.
run "split_600_spf" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = join("", ["v=spf1 ", join("", [for i in range(593) : "x"])])
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      join("\" \"", [
        join("", ["v=spf1 ", join("", [for i in range(248) : "x"])]),
        join("", [for i in range(255) : "x"]),
        join("", [for i in range(90) : "x"]),
      ]),
    ])
    error_message = "A 600-character SPF value must plan and split into 255 + 255 + 90."
  }
}

# REQ-12: an already-split value whose segments fit is byte-identical
# (splitting the output of a split again changes nothing).
run "split_idempotent" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = join("\" \"", [join("", ["v=spf1 ", join("", [for i in range(248) : "x"])]), "x"])
      apex_txt_values = [
        join("\" \"", [join("", [for i in range(255) : "y"]), join("", [for i in range(255) : "y"]), "y"]),
        "aaa\" \"bbb",
      ]
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      join("\" \"", [join("", ["v=spf1 ", join("", [for i in range(248) : "x"])]), "x"]),
      join("\" \"", [join("", [for i in range(255) : "y"]), join("", [for i in range(255) : "y"]), "y"]),
      "aaa\" \"bbb",
    ])
    error_message = "Already-split values with segments within 255 must be byte-identical."
  }
}

# REQ-12: an already-split value with a 256-character segment followed by abc.
run "split_oversized_segment" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      apex_txt_values = [join("\" \"", [join("", [for i in range(256) : "y"]), "abc"])]
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      "v=spf1 include:amazonses.com ~all",
      join("\" \"", [join("", [for i in range(255) : "y"]), "y", "abc"]),
    ])
    error_message = "A 256-character segment must split into 255 + 1, then abc."
  }
}

# REQ-12: an empty segment between two delimiters is dropped.
# The table's second row, a value that begins with the delimiter (`" "a`), is
# not tested as "dropped to a": the later plan-review rule refuses a leading
# `"`, and the refusal section below holds that case.
run "split_drops_empty_segments" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      apex_txt_values = ["a\" \"\" \"b"]
    }]
  }

  assert {
    condition = aws_route53_record.spf["example.com"].records == toset([
      "v=spf1 include:amazonses.com ~all",
      "a\" \"b",
    ])
    error_message = "Empty segments must be dropped."
  }
}

# REQ-14: manual instructions, no zone_id, two extras: order and notes.
run "manual_instructions_with_extras" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      spf_record      = join("", ["v=spf1 ", join("", [for i in range(249) : "x"])])
      apex_txt_values = ["google-site-verification=abc", join("", [for i in range(256) : "y"])]
    }]
  }

  assert {
    condition     = length(aws_route53_record.spf) == 0
    error_message = "A domain without zone_id must create no apex record."
  }
  assert {
    condition     = length(output.manual_dns_instructions["example.com"].records) == 10
    error_message = "Expected verification, SPF, 2 extras, DKIM x3, DMARC, MAIL FROM MX, MAIL FROM SPF."
  }
  assert {
    condition     = output.manual_dns_instructions["example.com"].records[1].note == "SPF authorization record"
    error_message = "SPF must be second."
  }
  assert {
    condition     = output.manual_dns_instructions["example.com"].records[1].value == join("", ["v=spf1 ", join("", [for i in range(248) : "x"]), "\" \"", "x"])
    error_message = "The manual SPF value must be the split effective value."
  }
  assert {
    condition = (
      output.manual_dns_instructions["example.com"].records[2].name == "example.com" &&
      output.manual_dns_instructions["example.com"].records[2].type == "TXT" &&
      output.manual_dns_instructions["example.com"].records[2].note == "Apex TXT value" &&
      output.manual_dns_instructions["example.com"].records[2].value == "google-site-verification=abc"
    )
    error_message = "The first extra value must be third, at the apex, TXT, note Apex TXT value."
  }
  assert {
    condition = (
      output.manual_dns_instructions["example.com"].records[3].name == "example.com" &&
      output.manual_dns_instructions["example.com"].records[3].type == "TXT" &&
      output.manual_dns_instructions["example.com"].records[3].note == "Apex TXT value" &&
      output.manual_dns_instructions["example.com"].records[3].value == join("\" \"", [join("", [for i in range(255) : "y"]), "y"])
    )
    error_message = "The second extra value must be fourth, split."
  }
  assert {
    condition     = output.manual_dns_instructions["example.com"].records[9].note == "Custom MAIL FROM SPF record"
    error_message = "MAIL FROM SPF must be last."
  }
  assert {
    condition     = output.manual_dns_instructions["example.com"].records[9].value == "v=spf1 include:amazonses.com ~all"
    error_message = "MAIL FROM SPF must keep the default value."
  }
  assert {
    condition = length([
      for r in output.manual_dns_instructions["example.com"].records : r
      if r.note == "Apex TXT value"
    ]) == 2
    error_message = "Exactly one manual entry per extra value."
  }
}

# REQ-14: with no extra values the manual output has no apex TXT entries.
run "manual_instructions_without_extras" {
  command = plan
  variables {
    domains = [{ domain = "example.com", apex_txt_values = [] }]
  }

  assert {
    condition     = length(output.manual_dns_instructions["example.com"].records) == 8
    error_message = "Without extras: verification, SPF, DKIM x3, DMARC, MAIL FROM MX, MAIL FROM SPF."
  }
  assert {
    condition = length([
      for r in output.manual_dns_instructions["example.com"].records : r
      if r.note == "Apex TXT value"
    ]) == 0
    error_message = "No Apex TXT value entries without extras."
  }
}

# ---------------------------------------------------------------------------
# Refusals (REQ-13 and the contract's validation list). Last in this file.
# ---------------------------------------------------------------------------

run "refuses_null_entry" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = [null] }]
  }
  expect_failures = [var.domains]
}

run "refuses_empty_entry" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = [""] }]
  }
  expect_failures = [var.domains]
}

run "refuses_delimiter_only_entry" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["\" \""] }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_spf_lowercase" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["v=spf1 -all"] }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_spf_uppercase" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["V=SPF1 -all"] }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_spf_mixed_case_bare" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["v=Spf1"] }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_spf_split_by_delimiter" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["v=sp\" \"f1 -all"] }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_bare_quote_pair" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["v=sp\"\"f1 -all"] }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_wrapped_in_quotes" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["\"v=spf1 -all\""] }]
  }
  expect_failures = [var.domains]
}

run "refuses_spf_record_wrapped_in_quotes" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "\"v=spf1 -all\"" }]
  }
  expect_failures = [var.domains]
}

run "refuses_spf_record_bare_quote_pair" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "v=spf1 a\"\"b -all" }]
  }
  expect_failures = [var.domains]
}

# Contract table row `" "a` -> `a` conflicts with the later rule that refuses a
# leading `"`; the later rule is taken here.
run "refuses_extra_leading_delimiter" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["\" \"a"] }]
  }
  expect_failures = [var.domains]
}

run "refuses_spf_record_newline" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "v=spf1 -all\n" }]
  }
  expect_failures = [var.domains]
}

run "refuses_spf_record_non_ascii" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", spf_record = "v=spf1 include:é.example.com -all" }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_newline" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["abc\ndef"] }]
  }
  expect_failures = [var.domains]
}

run "refuses_extra_non_ascii" {
  command = plan
  variables {
    domains = [{ domain = "example.com", zone_id = "Z000000000000A", apex_txt_values = ["café"] }]
  }
  expect_failures = [var.domains]
}

# A chunk ending in a lone backslash: 254 characters, then "\" at position 255.
run "refuses_escape_cut_backslash" {
  command = plan
  variables {
    domains = [{
      domain     = "example.com"
      zone_id    = "Z000000000000A"
      spf_record = join("", ["v=spf1 ", join("", [for i in range(247) : "x"]), "\\", "abc"])
    }]
  }
  expect_failures = [var.domains]
}

# A chunk ending in "\d": the decimal escape \065 cut after its first digit.
run "refuses_escape_cut_backslash_digit" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      apex_txt_values = [join("", [join("", [for i in range(253) : "y"]), "\\065", "abc"])]
    }]
  }
  expect_failures = [var.domains]
}

# A chunk ending in "\dd": the decimal escape \065 cut after its second digit.
run "refuses_escape_cut_backslash_two_digits" {
  command = plan
  variables {
    domains = [{
      domain          = "example.com"
      zone_id         = "Z000000000000A"
      apex_txt_values = [join("", [join("", [for i in range(252) : "y"]), "\\065", "abc"])]
    }]
  }
  expect_failures = [var.domains]
}
