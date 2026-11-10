# Environment Policy

Generated Terraform reads flags. Only `policyForEnvName` in `app/env_policy.go`
looks at an environment's name, and it runs when meroku creates an environment
(`createEnv`) or migrates a YAML file (v31). The result is written into the YAML,
where a team can change it.

| Name | `is_prod` | `auto_deploy` | `domain.add_env_domain_prefix` |
|---|---|---|---|
| `dev` | `false` | `true` | absent (prefixed) |
| `prod`, `production` | `true` | `false` | `false` |
| anything else | `false` | `false` | absent (prefixed) |

`TestTerraformDoesNotBranchOnTheEnvName` fails if `modules/**.tf` or `env/main.hbs`
compares against the name again.

Go code that mirrors DNS or SES naming (`app/ses_primary_domain.go`,
`app/ses_zone_origin.go`, `app/dns_preflight.go`) still reads the name.

## Auto-deploy

The top-level `auto_deploy` is the default for the backend, services and scheduled
tasks. Each can override it. It decides whether an event may deploy a target. A
manual deploy always works. Migration v31 turns every target off in any environment
not named `dev`.

## Sharing a registry

An environment that uses another environment's images needs three connections.

| Connection | Source YAML | This YAML |
|---|---|---|
| Pull access | `ecr_trusted_accounts[]` | |
| Image location | | `ecr_strategy: cross_account`, `ecr_account_id`, `ecr_account_region` |
| Push events | `forward_image_events: true` on the entry | `ecr_image_events: true` |

```yaml
# dev.yaml
ecr_strategy: local
ecr_trusted_accounts:
  - account_id: "222222222222"
    env: sandbox
    region: us-east-1
    forward_image_events: true

# sandbox.yaml
ecr_strategy: cross_account
ecr_account_id: "111111111111"
ecr_account_region: us-east-1
ecr_image_events: true
```

The web UI's cross-account ECR selection sets all of these.

Apply the receiving environment first. Its event bus must allow the source account
before the source's forwarding rule can deliver anything, and a refused delivery
shows up only in the rule's `FailedInvocations` metric.

Forwarded events keep `source = aws.ecr`, so the receiving environment's ordinary
`ci_ecr_push` rule handles them and the CI Lambda applies `auto_deploy` as usual.
In the same account and region there is no forwarding, because both environments
already read the same default bus.

Under `cross_account` nothing is created here. The backend, every task and every
service with `ecr_config.mode: create_ecr` (the default) pull `<project>_backend`,
`<project>_task_<name>` and `<project>_service_<name>` from the source registry, so
the source environment must define the same services and tasks. `manual_repo` and
`use_existing` keep their meaning in both strategies.

Switching an environment to `cross_account` deletes its own repositories and the
images in them (`force_delete`).
