# Every capped AWS name this module builds, in one place. See modules/naming.
#
# IAM roles and EventBridge rules both cap at 64. The rule name additionally
# carries a per-rule key, so it has two user-supplied fields competing for the
# same budget.
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = merge(
    {
      task_role = {
        legacy    = "${var.project}_${var.task}_task_${var.env}"
        parts     = [var.task, "task"]
        limit     = 64
        separator = "_"
      }
      task_execution_role = {
        legacy    = "${var.project}_scheduler_${var.task}_task_execution_${var.env}"
        parts     = ["scheduler", var.task, "task", "execution"]
        limit     = 64
        separator = "_"
      }
    },
    { for k in keys(local.all_rules) : "rule_${k}" => {
      legacy    = "${var.project}_rule_${k}_${var.env}"
      parts     = ["rule", k]
      limit     = 64
      separator = "_"
    } },
  )
}
