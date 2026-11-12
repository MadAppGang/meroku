# The user's own IAM statements (workload.policy, services[].policy), each
# workload's as one customer-managed policy on its task role.
#
# Managed rather than inline: a user policy can be large, a managed policy has
# its own 6,144-character limit outside the role's 10,240-character inline
# budget, and these roles are far below the quota of 10 attached policies.
#
# Each YAML statement becomes one IAM statement:
#
#   { Effect, Action, Resource }                 no conditions
#   { Effect, Action, Resource, Condition }      conditions present
#
# Condition is grouped by test, then keyed by variable, as IAM expects:
#
#   conditions:                                  Condition = {
#     - { test: StringEquals, variable: a,         StringEquals = { a = [..], b = [..] }
#         values: [..] }                           Bool         = { c = [..] }
#     - { test: StringEquals, variable: b, .. }  }
#     - { test: Bool, variable: c, .. }
#
# The `...` grouping keeps two conditions that share a test; merge() then
# folds their variables into one object. app/workload_policy.go refuses two
# conditions with the same test and variable, which merge() would collapse.
#
# The Condition key is added with a `for` filter rather than
# `cond ? { Condition = .. } : {}`: the two arms of a conditional must have one
# type, and these two objects do not. A statement without conditions therefore
# has exactly the keys it had before conditions existed, and jsonencode orders
# keys, so its document is byte-identical.
locals {
  backend_custom_policy_statements = [
    for s in var.backend_policy : merge(
      { Effect = s.effect, Action = s.actions, Resource = s.resources },
      { for k, v in {
        Condition = {
          for test, entries in { for c in s.conditions : c.test => { (c.variable) = c.values }... } :
          test => merge(entries...)
        }
      } : k => v if length(s.conditions) > 0 },
    )
  ]

  service_custom_policy_statements = {
    for name, statements in var.service_policies : name => [
      for s in statements : merge(
        { Effect = s.effect, Action = s.actions, Resource = s.resources },
        { for k, v in {
          Condition = {
            for test, entries in { for c in s.conditions : c.test => { (c.variable) = c.values }... } :
            test => merge(entries...)
          }
        } : k => v if length(s.conditions) > 0 },
      )
    ]
  }

  # Only services that exist and have at least one statement get a policy. A
  # key naming no service is dropped rather than failing on the role lookup.
  services_with_custom_policy = toset([
    for name, statements in var.service_policies : name
    if length(statements) > 0 && contains(keys(local.service_names), name)
  ])
}

resource "aws_iam_policy" "services_custom" {
  for_each = local.services_with_custom_policy

  name = "${var.project}_service_${each.key}_custom_policy_${var.env}"
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = local.service_custom_policy_statements[each.key]
  })

  tags = {
    Name        = "${var.project}_service_${each.key}_custom_policy_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "services_custom" {
  for_each = local.services_with_custom_policy

  role       = aws_iam_role.services_task[each.key].name
  policy_arn = aws_iam_policy.services_custom[each.key].arn
}
