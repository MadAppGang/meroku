# SES DNS ownership

Which DNS values meroku writes for an SES domain, and which it leaves alone,
depends on who created the Route53 zone the domain lives in.

## Zone origin

`resolveSESZones` (`app/ses_zone_origin.go`) decides it at generate time. It
follows the same order the template uses to pick the `zone_id`:

| The domain's zone comes from | Origin |
|---|---|
| `ses.domains[].zone_id` | existing |
| `domain.additional_domains[]` entry with `zone_id` | existing |
| `domain.additional_domains[]` entry with `create_zone: true` | created |
| the main domain, outside prod, with `domain.root_zone_id` (delegated `<env>.` zone) | created |
| the main domain, `domain.create_domain_zone: true` | created |
| the main domain otherwise | existing |
| none of the above | no zone: records are listed for manual setup |

The module input is `zone_origin` per domain (`zone_origin` at the top level
for the legacy `ses.domain_name` form). It defaults to `"created"`, and the
template prints it only for existing zones. An env whose SES domains all sit
in zones meroku created therefore generates the same Terraform as v4.11.1;
`app/ses_zone_origin_test.go` pins that with goldens.

## Created zones: the YAML is the whole truth

Records are `aws_route53_record` resources with `allow_overwrite = true`. Each
one replaces the whole record set at its name on apply and deletes it on
destroy:

- apex TXT: `spf_record` (default `v=spf1 include:amazonses.com ~all`) plus
  `apex_txt_values`, TTL `spf_ttl` (default 600);
- `_amazonses.<domain>` verification TXT, `_dmarc.<domain>` DMARC TXT;
- every `txt_records[]` entry, one record set per name.

A value that is not in the YAML is removed on the next apply.

## Existing zones: meroku touches only its own values

Route53 has one TXT record set per name, shared with whoever else uses the
zone. meroku edits it value by value with `modules/ses/scripts/route53-txt.sh`,
run from `terraform_data` resources through `local-exec`:

| Resource | Key | What it owns |
|---|---|---|
| `terraform_data.dns_slot` | `<domain>\|@\|spf`, `<domain>\|_dmarc\|dmarc` | The single sender (SPF) value at the apex, the single DMARC value at `_dmarc`. Adding replaces any other value of that kind. |
| `terraform_data.dns_value` | `<domain>\|@\|<hash>`, `<domain>\|<name>\|<hash>`, `<domain>\|_amazonses\|verification` | One `apex_txt_values` value, one `txt_records` value, the verification token. Added beside whatever is there. |
| `terraform_data.dns_ttl` | `<domain>\|@`, `<domain>\|<name>` | The TTL of the set, only when `spf_ttl` or a record's `ttl` is set. Unset keeps the live TTL. |

Each change is one Route53 batch that deletes the exact set it read and
creates the new one, so a concurrent change makes the batch fail and the
script reads again (up to six attempts). DKIM CNAMEs and the MAIL FROM
records stay `aws_route53_record`: their names belong to SES alone.

Requirements where Terraform runs: a POSIX `/bin/sh` and AWS CLI v2. The
script passes no `--profile`; it uses the same credentials as the provider.

### The takeover guard

A domain can publish only one SPF record and one DMARC record. Before
replacing one meroku did not write, plan reads the live values
(`data.aws_route53_records.live`) and refuses unless the YAML says what to
publish instead:

- SPF: set `spf_record`. The error message suggests the live value with
  `include:amazonses.com` added; the web app offers the same value as
  "This app and the services your domain already uses".
- DMARC: set `dmarc_policy` or `dmarc_rua_email` for the domain.

When the YAML does say, the check `ses_dns_slot_replacements` warns with the
values that apply will replace.

### Destroy

- Removing a value or a record from the YAML removes only that value.
- Removing the domain, or disabling SES (`removed` blocks with a destroy
  provisioner, rendered in the `{{else}}` of `ses.enabled`), removes only
  meroku's values. Everything else in the set stays.
- A foreign SPF or DMARC value meroku replaced is not restored.

## Moving an existing zone from v4.11.1

v4.11.1 managed these sets as `aws_route53_record`. For each domain in an
existing zone the template renders, between
`# SES records in DNS zones meroku did not create` and its end marker:

- `moved` blocks from `spf`, `domain_amazonses_verification_record` and
  `dmarc` to `*_released` addresses the module does not declare;
- `removed { lifecycle { destroy = false } }` blocks for those addresses.

Plan shows them as **forget**: they leave state and stay in Route53. The plan
viewer shows `⊘ FORGET (kept in AWS)`, never a deletion. The new
`terraform_data` resources then adopt the values in place.

## Value rules

Checked at generate by `validateSESDNSConfigMap` (`app/ses_dns_validate.go`),
again at plan by the module's variable validations, and as you type in the
web app (`web/src/lib/sesDnsValidation.ts`, a mirror of the Go rules):

- `spf_record` starts with `v=spf1`; no SPF value anywhere else;
- printable ASCII, no `"` except the `" "` delimiter, no split inside a `\`
  escape; values over 255 characters are split automatically;
- TTLs are whole numbers, 0 or more;
- `txt_records[].name` is relative to the domain, a valid DNS name, not `@`,
  not `_amazonses`, not `_dmarc`, not the MAIL FROM subdomain, and used once.

## Web app

`GET /api/ses-dns?env=&domain=[&candidate=][&live=0]` (`app/api_ses_dns.go`)
returns the zone origin, the live apex values, the suggested sender record
and the YAML's issues. The SES panel's "Email sender records" section shows
the ownership note for the origin, the sender choices, the TTL, and the
"Other text records at the domain" and "Extra text records" editors. Edits
reach the YAML only once they pass the rules above.

## Tests

- `sh modules/ses/scripts/test/run.sh`: the script against a fake `aws`.
- `sh modules/ses/tests/lifecycle/run.sh` (in `modules/ses`): add, change,
  remove and destroy through Terraform against the fake `aws`, with foreign
  values seeded and checked after the final destroy.
- `terraform -chdir=modules/ses test`: plan-level tests for both origins.
- `go test ./app/...`: goldens, resolver, template, validation, API.
