# meroku handoff: {{.Operation}}

meroku started you to help with a problem in this project. This file is the whole
briefing: what meroku is, where it ran, what failed, and the rules for fixing it.
Read all of it before running anything.

## Goal

{{.Goal}}

## THE RULES

These rules come from the user. They apply for the whole session.

1. **Do NOT fix the problem directly in AWS.** No console changes and no AWS CLI or
   SDK writes (`create-*`, `put-*`, `update-*`, `delete-*`, `attach-*`, `tag-*`, …).
   A change made in AWS is not in the YAML or the Terraform state, so the next meroku
   run either reverts it or fails on the drift.
2. **Do NOT edit the generated Terraform in `env/{{.EnvName}}/`.** meroku rewrites
   every file there from the YAML on each run, so a hand edit is lost on the next
   generate or drifts away from the YAML that produced it.
3. **Investigate and explain first; ask before any write.** Tell the user what you
   found and what you propose, and wait for a yes before you change a file, run a
   command that changes AWS or Terraform state, or rerun meroku.

Read-only inspection is fine at any time:

- `aws ... describe-*`, `list-*`, `get-*`, `aws sts get-caller-identity`
- `terraform state list`, `terraform state show <address>`, `terraform plan`
  (run from `env/{{.EnvName}}/`; a plan without `-out` changes nothing)
- reading any file in the project

Use the profile from the env YAML, not whatever your shell has: prefix AWS and
Terraform commands with `AWS_PROFILE={{.ProfileArg}}`. meroku exports that profile
itself before it runs Terraform; this pane may not have inherited it.

### Allowed fixes, in this order

(a) **The cause is a meroku bug** — a template, module, migration or the meroku
    binary does the wrong thing for a valid configuration. Say so, with evidence:
    the file and line, and the input that triggers it. The fix belongs in meroku
    itself (github.com/MadAppGang/meroku). Ask the user for the path to their local
    checkout of it and make the change there. `infrastructure/` in this project is a
    copy of meroku's templates and modules that meroku replaces on update; editing it
    is at most a stop-gap, and only if the user asks for one.

(b) **An edge case meroku cannot migrate by itself** — for example a resource that
    already exists under another name, state that has to move, or something created
    outside meroku. Interview the user first: what they expect, what is deployed,
    what may be lost. Then do the one-off manual step together with them, each
    command stated and agreed before it runs, and write down exactly what was done
    (commands and results) so it can be repeated or audited.

(c) **A configuration problem** — the YAML asks for something invalid or
    inconsistent. Change the parameters in the env YAML (see "How meroku works"
    below), then rerun meroku so it regenerates and applies.

If none of these fits, stop and ask the user.

## Where it ran

| | |
|---|---|
| Operation | {{.Operation}} |
| Environment | {{.EnvDisplay}} |
| Project root | {{cell .ProjectRootDisplay}} |
| AWS account | {{.AccountDisplay}} |
| AWS profile | {{.ProfileDisplay}} |
| AWS region | {{.RegionDisplay}} |
| meroku version | {{.VersionDisplay}} |
| Time | {{.TimeDisplay}} |

The profile, region and account come from the env YAML meroku loaded, not from the
shell environment.

## What failed
{{if not .HasFailureData}}
meroku recorded no failure details for this handoff. Ask the user what is wrong.
{{end}}{{if .Failures}}
### Failed resources
{{range $i, $f := .Failures}}
{{inc $i}}. `{{or $f.Address "(no address)"}}`{{if $f.Action}} — action: {{$f.Action}}{{end}}
{{- if $f.Summary}}
   - Summary: {{$f.Summary}}{{end}}
{{- if $f.Location}}
   - Location: {{$f.Location}}{{end}}
{{- if $f.Detail}}
   - Detail:

{{indent $f.Detail}}{{end}}
{{end}}{{end}}{{if .Diagnostics}}
### Diagnostics without a resource address

Provider, backend and configuration errors that Terraform did not tie to a resource.
{{range .Diagnostics}}
{{fence .}}
{{end}}{{end}}{{if .ErrorOutput}}
### Error output{{if .Truncated}} (tail){{end}}

{{fence .ErrorOutput}}
{{end}}
## How meroku works

meroku turns one YAML file per environment into Terraform and runs it. It always runs
from the project root ({{code .ProjectRootDisplay}}), never from the meroku source tree.

1. **YAML.** The environment's config is `{{.EnvName}}.yaml` at the project root or
   `project/{{.EnvName}}.yaml`, searched in that order. This is the file to change for
   a configuration fix.
2. **Migrations.** On load, meroku upgrades the YAML to the current schema version
   (`schema_version` in the file) and, before saving, writes a timestamped backup to
   a `backup/` directory beside it (`backup/<file>.backup_<timestamp>`).
3. **Templates.** The YAML values render the Handlebars template
   `infrastructure/env/main.hbs`, which calls the Terraform modules in
   `infrastructure/modules/` (referenced as `../../infrastructure/modules`). User-owned
   Terraform in `custom/terraform/_shared/` and `custom/terraform/{{.EnvName}}/` is
   appended as is.
4. **Generated Terraform.** The result is written to `env/{{.EnvName}}/*.tf`. Generated,
   never edited by hand.
5. **Terraform.** meroku runs `terraform init`, `plan` and `apply` (or `destroy`) inside
   `env/{{.EnvName}}/`. State lives in the S3 backend named by `state_bucket` /
   `state_file` in the YAML.

## How to rerun

Ask the user to run these, or run them yourself only after they agree:

- `meroku generate {{.EnvName}}` — regenerate `env/{{.EnvName}}/` from the YAML. Touches
  only local files.
- `meroku --env {{.EnvName}}{{if .AWSProfile}} --profile {{.ProfileArg}}{{end}}`, then
  "🚀 Deploy environment" — generates, runs the AWS pre-flight checks, initialises
  Terraform if needed, shows the plan and applies after confirmation.
- `meroku --env {{.EnvName}}`, then "💥 Nuke/Destroy Environment" — the destroy flow.
- `meroku migrate <path to the YAML>` (`{{.EnvName}}.yaml` or `project/{{.EnvName}}.yaml`),
  or `meroku migrate all` — run the schema migrations on their own.
- `meroku sync {{.EnvName}}` — check the environment against its S3 state and reconnect a
  disconnected `env/{{.EnvName}}/`. Read-only.
- `meroku dns status`, `meroku dns validate` — read-only DNS checks.
