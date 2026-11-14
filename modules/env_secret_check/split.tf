# The same question as main.tf — would ECS refuse this container definition? —
# asked in two parts, split by when the answer can be known.
#
# A precondition whose condition is unknown at plan is postponed to apply. The
# secrets discovered under a workload's own SSM path can be unknown at plan (a
# data source that depends on a resource being created or replaced is read during
# apply), so one condition built from every secret would quietly move every check
# to apply whenever that happens. Hence two pairs:
#
#   valid_known / message_known — built only from `environment`,
#   `extra_env_names` and `known_secrets`, all known at plan. Always decided at
#   plan, on a fresh workload too.
#     1. No name meroku adds through extra_env is in `environment` more than
#        once. Duplicates among the module's own defaults and the YAML variables
#        are NOT refused: a YAML variable may follow a default of the same name,
#        which the task modules document as the way to override it.
#     2. No known secret shares a name with an environment variable or with
#        another known secret (vault against vault, vault against ENV).
#
#   valid_discovered / message_discovered — check 3: no discovered secret shares
#   a name with an environment variable or with a known secret. Decided at plan
#   when the own-path read is, at apply otherwise.
#
# Callers put one precondition on each pair, in the one lifecycle block of the
# task definition.

locals {
  known_names = { for k, w in var.workloads : k => [for s in w.known_secrets : s.name] }

  # Check 1.
  extra_env_duplicates = { for k, w in var.workloads : k => sort(distinct([
    for n in w.extra_env_names : n if length([for e in w.environment : e if e == n]) > 1
  ])) }

  # Check 2.
  known_env_overlap = { for k, w in var.workloads : k =>
    sort(tolist(setintersection(toset(w.environment), toset(local.known_names[k]))))
  }
  known_duplicates = { for k, w in var.workloads : k => sort(distinct([
    for n in local.known_names[k] : n if length([for m in local.known_names[k] : m if m == n]) > 1
  ])) }

  # Check 3.
  discovered_overlap = { for k, w in var.workloads : k => sort(distinct([
    for s in w.discovered_secrets : s.name if contains(w.environment, s.name) || contains(local.known_names[k], s.name)
  ])) }

  valid_known = { for k, w in var.workloads : k =>
    length(local.extra_env_duplicates[k]) == 0 && length(local.known_env_overlap[k]) == 0 && length(local.known_duplicates[k]) == 0
  }
  valid_discovered = { for k, w in var.workloads : k => length(local.discovered_overlap[k]) == 0 }

  # The origins of each known secret name, joined for a sentence.
  known_origins = { for k, w in var.workloads : k => {
    for n in distinct(local.known_names[k]) : n => join(" and ", [for s in w.known_secrets : s.origin if s.name == n])
  } }

  # Every message below is total: it renders for a clean workload too, because a
  # precondition's message is rendered for every instance.
  known_problems = { for k, w in var.workloads : k => concat(
    [for n in local.extra_env_duplicates[k] :
      "- ${n} is set more than once in the environment. meroku sets it for this workload itself; the other copy comes from ${w.yaml_field} in ${w.yaml_file}. Remove it there and read meroku's value."
    ],
    [for n in local.known_env_overlap[k] :
      "- ${n} is a plain environment variable (from ${w.yaml_field} in ${w.yaml_file}, ${w.defaults_source} or a meroku feature) and also the secret from ${local.known_origins[k][n]}. Keep exactly one: remove the variable, or connect the vault with a prefix so the secret gets another name."
    ],
    [for n in local.known_duplicates[k] :
      "- ${n} would be set by more than one secret: ${local.known_origins[k][n]}. Give the connected vaults different prefixes, or rename one of the secrets in its vault's secrets list."
    ],
    contains(local.known_env_overlap[k], "ENV") || contains(local.known_duplicates[k], "ENV") ? [
      "ENV is reserved. meroku creates ${w.ssm_path}/env itself and sets it as the secret ENV. A vault secret named env, connected without a prefix, becomes ENV as well; connect that vault with a prefix (prefix PAY makes it PAY_ENV, which is allowed) or rename the secret in the vault's secrets list. An ENV in ${w.yaml_field} is removed from the YAML and set with: aws ssm put-parameter --name ${w.ssm_path}/env --value <value> --overwrite"
    ] : [],
  ) }

  message_known = { for k, w in var.workloads : k => length(local.known_problems[k]) == 0 ? "${w.subject} has no name collision among its environment variables, ENV and its vault secrets." : join("\n\n", concat(
    ["${w.subject} cannot be registered: ECS refuses a container definition that sets one name twice, or as both a variable and a secret. This was found at plan, before anything changed."],
    local.known_problems[k],
  )) }

  discovered_problems = { for k, w in var.workloads : k => [
    for n in local.discovered_overlap[k] : "- ${n} is a secret from ${join(" and ", [for s in w.discovered_secrets : s.origin if s.name == n])}, under the workload's own path ${w.ssm_path}, and is also ${join(" and ", concat(
      contains(w.environment, n) ? ["a plain environment variable (from ${w.yaml_field} in ${w.yaml_file}, ${w.defaults_source} or a meroku feature)"] : [],
      contains(local.known_names[k], n) ? ["the secret from ${local.known_origins[k][n]}"] : [],
    ))}. Keep exactly one: delete or rename the parameter under ${w.ssm_path}, or remove the other."
  ] }

  message_discovered = { for k, w in var.workloads : k => length(local.discovered_problems[k]) == 0 ? "${w.subject} has no name collision between the secrets under ${w.ssm_path} and its other variables and secrets." : join("\n\n", concat(
    ["${w.subject} cannot be registered: a secret found under its own SSM path has the name of another variable or secret, and ECS refuses a container definition that sets one name twice."],
    local.discovered_problems[k],
  )) }
}

output "valid_known" {
  description = "Per workload: checks 1 and 2 pass. Built only from values known at plan."
  value       = local.valid_known
}

output "message_known" {
  description = "Per workload: the sentence a precondition shows when `valid_known` is false. Total."
  value       = local.message_known
}

output "valid_discovered" {
  description = "Per workload: check 3 passes. Unknown at plan while the own-path read is."
  value       = local.valid_discovered
}

output "message_discovered" {
  description = "Per workload: the sentence a precondition shows when `valid_discovered` is false. Total."
  value       = local.message_discovered
}
