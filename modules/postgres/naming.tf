# Every AWS name this module builds, in one place. See modules/naming.
#
# An RDS identifier is capped at 63 characters. These three are short enough
# today for any sane project, but the cap is real and the module makes it
# structural rather than a thing someone has to remember.
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
    postgres = {
      legacy = "${var.project}-postgres-${var.env}"
      parts  = ["postgres"]
      limit  = 63
    }
    aurora = {
      legacy = "${var.project}-aurora-${var.env}"
      parts  = ["aurora"]
      limit  = 63
    }
    aurora_instance = {
      legacy = "${var.project}-aurora-instance-${var.env}"
      parts  = ["aurora", "instance"]
      limit  = 63
    }
  }
}
