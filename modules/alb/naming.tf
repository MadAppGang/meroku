# Every AWS name this module builds, in one place. See modules/naming.
#
# An ALB name is capped at 32 characters, which "${project}-alb-${env}" reaches
# once the project is 24 characters or more.
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
    alb = {
      legacy = "${var.project}-alb-${var.env}"
      parts  = ["alb"]
      limit  = 32
    }
  }
}
