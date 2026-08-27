# Every capped AWS name this module builds, in one place. See modules/naming.
#
# IAM roles cap at 64 and EventBridge Scheduler at 64. `task` is user-supplied
# from the environment YAML, so these are the templates most exposed to a long
# name: "${project}_scheduler_${task}_task_execution_${env}" spends 32
# characters on decoration before the project or the task get a say.
#
# The IAM *policies* below are capped at 128 and are nowhere near it, so they
# stay as literal templates rather than adding noise here.
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
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
    scheduler_role = {
      legacy    = "${var.project}_scheduler_${var.task}_role_${var.env}"
      parts     = ["scheduler", var.task, "role"]
      limit     = 64
      separator = "_"
    }
    # env sits in the middle of this legacy form. Form 2 would move it to the
    # end, which only ever happens once the legacy form no longer fits.
    schedule_group = {
      legacy = "${var.project}-schedule-group-${var.env}-${var.task}"
      parts  = ["schedule", "group", var.task]
      limit  = 64
    }
    schedule = {
      legacy = "${var.project}-scheduler-${var.task}-${var.env}"
      parts  = ["scheduler", var.task]
      limit  = 64
    }
  }
}
