# Every capped AWS name this module builds, in one place. See modules/naming.
#
# IAM roles and Lambda function names both cap at 64. These legacy forms put env
# in the middle, which form 2 would move to the end; legacy wins while it fits,
# so that reordering only ever reaches AWS for a project too long for the
# decorated form.
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
    appsync_role = {
      legacy = "${var.project}-${var.env}-appsync-role"
      parts  = ["appsync", "role"]
      limit  = 64
    }
    lambda_exec_role = {
      legacy = "${var.project}-${var.env}-appsync-lambda-exec"
      parts  = ["appsync", "lambda", "exec"]
      limit  = 64
    }
    auth_lambda = {
      legacy = "${var.project}-${var.env}-appsync-auth"
      parts  = ["appsync", "auth"]
      limit  = 64
    }
  }
}
