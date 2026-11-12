# The task's own IAM statements (event_processor_tasks[].policy), as one
# customer-managed policy on the task role. Same shape and the same statement
# expression as workload.policy in modules/workloads/custom_policy.tf, where
# the Condition grouping is explained.
#
# Managed rather than inline: a user policy can be large, a managed policy has
# its own 6,144-character limit outside the role's inline budget, and the task
# role is far below the quota of 10 attached policies.

variable "custom_policy" {
  description = "Custom IAM statements for the task role, created as one customer-managed policy. Empty creates nothing."
  type = list(object({
    effect    = optional(string, "Allow")
    actions   = list(string)
    resources = list(string)
    conditions = optional(list(object({
      test     = string
      variable = string
      values   = list(string)
    })), [])
  }))
  default = []

  validation {
    condition     = alltrue([for s in var.custom_policy : contains(["Allow", "Deny"], s.effect)])
    error_message = "custom_policy[].effect must be Allow or Deny."
  }
}

locals {
  custom_policy_statements = [
    for s in var.custom_policy : merge(
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

resource "aws_iam_policy" "custom" {
  count = length(var.custom_policy) > 0 ? 1 : 0

  name = "${var.project}_task_${var.task}_custom_policy_${var.env}"
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = local.custom_policy_statements
  })

  tags = {
    Name        = "${var.project}_task_${var.task}_custom_policy_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "custom" {
  count = length(var.custom_policy) > 0 ? 1 : 0

  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.custom[0].arn
}
