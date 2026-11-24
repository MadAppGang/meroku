# Every AWS name this module builds, in one place. See modules/naming.
#
# An RDS identifier is capped at 63 characters and a DB subnet group at 255. The
# names are short enough for any sane project, but the cap is real and the module
# makes it structural rather than a thing someone has to remember.
#
# Public and private subnet groups have different names on purpose: the
# postconditions in main.tf compare the name to tell that an existing database
# would have to move.
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
    postgres_subnet = {
      parts = ["postgres", "subnet"]
      limit = 255
    }
    postgres_private_subnet = {
      parts = ["postgres", "private", "subnet"]
      limit = 255
    }
    aurora_subnet = {
      legacy = "${var.project}-aurora-subnet-${var.env}"
      parts  = ["aurora", "subnet"]
      limit  = 255
    }
    aurora_private_subnet = {
      parts = ["aurora", "private", "subnet"]
      limit = 255
    }
  }
}
