# Workload IAM policies (`policy`)

Every workload kind can carry its own IAM statements: the backend
(`workload.policy`), each service (`services[].policy`), each scheduled task
(`scheduled_tasks[].policy`) and each event task (`event_processor_tasks[].policy`).
meroku turns a workload's statements into **one customer-managed IAM policy**
attached to that workload's **task role**, the role the container's own code
assumes when it calls AWS.

## YAML

The shape is the same everywhere:

```yaml
services:
  - name: api
    policy:
      - actions: ["dynamodb:GetItem", "dynamodb:PutItem"]
        resources: ["arn:aws:dynamodb:us-east-1:000000000000:table/example"]
      - effect: Deny
        actions: ["s3:DeleteObject"]
        resources: ["*"]
        conditions:
          - test: StringNotEquals
            variable: aws:RequestedRegion
            values: [us-east-1]
```

| Field | Required | Meaning |
|---|---|---|
| `effect` | no | `Allow` or `Deny`, case as written. Absent means `Allow`. |
| `actions` | yes | Non-empty list of IAM actions. |
| `resources` | yes | Non-empty list of ARNs (or `*`). |
| `conditions[]` | no | `test` (an IAM condition operator such as `StringEquals`), `variable` (a condition key such as `aws:RequestedRegion`), `values` (non-empty list). |

IAM policy variables such as `${aws:username}` or `${aws:PrincipalTag/team}` are
written as they are and reach IAM literally; meroku escapes them for Terraform.

## What gets created

| Workload | Policy | Attached to |
|---|---|---|
| Backend | `aws_iam_policy.backend_custom_policy`, `<project>_backend_custom_policy_<env>` | `<project>_backend_task_<env>` |
| Service | `aws_iam_policy.services_custom["<name>"]`, `<project>_service_<name>_custom_policy_<env>` | the service's task role |
| Scheduled task | `module.schedule_task_<name>.aws_iam_policy.custom[0]`, `<project>_task_<name>_custom_policy_<env>` | the task role |
| Event task | `module.event_bus_task_<name>.aws_iam_policy.custom[0]`, same name pattern | the task role |

A workload with no statements gets no policy and no attachment.

**Managed, not inline.** Every other grant meroku adds to a role is inline, but
user policies are managed on every workload kind: a user policy can be large, a
managed policy has its own 6,144-character limit outside the role's
10,240-character inline budget, and these roles are far below the quota of 10
attached managed policies.

## How a statement renders

Each YAML statement becomes one IAM statement:

```json
{ "Action": [...], "Effect": "Allow", "Resource": [...] }
```

and, when it has conditions, a `Condition` key grouped by test and then by
variable, as IAM expects:

```yaml
conditions:
  - { test: StringEquals, variable: aws:RequestedRegion, values: [us-east-1] }
  - { test: StringEquals, variable: aws:PrincipalTag/team, values: [payments] }
  - { test: Bool, variable: aws:SecureTransport, values: ["true"] }
```

```json
"Condition": {
  "StringEquals": { "aws:RequestedRegion": ["us-east-1"], "aws:PrincipalTag/team": ["payments"] },
  "Bool": { "aws:SecureTransport": ["true"] }
}
```

A statement without conditions has no `Condition` key at all, so a policy
written before `effect` and `conditions` existed produces a byte-identical
document and an empty plan. The expression is
`local.*_custom_policy_statements` in `modules/workloads/custom_policy.tf` and
`modules/{ecs_task,event_bridge_task}/custom_policy.tf`.

## Validation (`meroku generate`)

`app/workload_policy.go` (`validateWorkloadPolicyConfigMap`, called from
`preprocessEnvMap`) refuses, naming the workload, the statement index and the
field:

- `policy` that is not a list, or a statement that is not a map;
- empty or missing `actions` or `resources`, or an entry that is not a non-empty string;
- an `effect` other than `Allow` or `Deny` (an empty string included; remove the key for Allow);
- a condition missing its `test`, `variable` or `values`;
- two conditions in one statement with the same `test` and `variable`. IAM keys
  a condition by both, so one of them would be lost; put every value in one condition.

It warns, without failing, on an action of `*` or `*:*`.

The Terraform variables repeat the `effect` rule as a variable validation.

## How service statements reach Terraform

The services list renders through `{{{array services}}}`, which does not escape
HCL template openers. A `${aws:username}` left in a service map would be a
Terraform interpolation in the generated `main.tf`, even though the module drops
the attribute during object conversion: HCL parses the string first.

So `preprocessEnvMap` validates every policy block, then `moveServicePoliciesOut`
moves each service's `policy` into a top-level `_service_policies` map keyed by
service name and deletes it from the service map. `env/main.hbs` renders it as
the `service_policies` input of `module.workloads`, one `hclArray` per service.
The backend renders `backend_policy = {{{hclArray workload.policy}}}` and each
task block `custom_policy = {{{hclArray policy}}}`.

## Web UI

The IAM tab of the backend, every service, every scheduled task and every event
task shows the same editor (`web/src/components/WorkloadPolicyEditor.tsx`): a
list of statements, each with an effect, actions and resources. Conditions are
shown as a count and are edited in YAML only; the editor carries them, and any
other field it does not show, through every edit. The list is read from the
config on every render and the panel is keyed by node, so a draft never follows
the user to another workload.

## Release note

Before this, `workload.policy` rendered only `actions` and `resources` with a
hard-coded `Effect = "Allow"`: a backend statement with `effect: Deny` deployed
as an Allow, and its `conditions` were dropped. It now deploys as written, which
changes the backend's managed policy in place (same address). Check backend
policies that set either field. A statement with empty `actions` or `resources`
is now refused at generate.
