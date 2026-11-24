# Where the database lands: which DB subnet group the standard instance and the
# Aurora cluster use, for every placement the module supports.
#
#   default VPC, standard   no subnet group; the account's default group is used
#   own VPC, standard       <project>-postgres-subnet-<env> on var.subnet_ids
#   own VPC, Aurora         <project>-aurora-subnet-<env> (the name it always had)
#   private, standard       <project>-postgres-private-subnet-<env> on private_subnet_ids
#   private, Aurora         <project>-aurora-private-subnet-<env> on private_subnet_ids
#
# An existing database never moves between public and private subnets: the
# group names differ by placement, and a postcondition on the instance or
# cluster refuses at plan a configuration that names a group other than the
# one in state.
#
# Ordering matters. Without state_key (CI runs Terraform 1.10.5) every run in
# this file shares one state:
#   - the plan-only shape runs come first, against an empty state;
#   - the flip runs then alternate apply (build a database) and plan (try to
#     move it), switching engine between pairs so each apply creates a fresh
#     database rather than updating the one before it;
#   - runs expected to fail a precondition come last.
#
# Every id below is synthetic; this repository is public.
#
# Run: terraform -chdir=modules/postgres test

mock_provider "aws" {}
mock_provider "random" {}

override_data {
  target = data.aws_vpc.selected
  values = { cidr_block = "10.42.0.0/16" }
}

variables {
  project    = "myapp"
  env        = "dev"
  vpc_id     = "vpc-00000000000000000"
  subnet_ids = ["subnet-aaa00000000000001", "subnet-aaa00000000000002"]
}

# ---------------------------------------------------------------------------
# Shapes, planned against an empty state
# ---------------------------------------------------------------------------

run "default_vpc_standard_has_no_subnet_group" {
  command = plan

  variables {
    default_vpc = true
  }

  assert {
    condition     = length(aws_db_subnet_group.database) == 0
    error_message = "In the default VPC the standard instance keeps the account's default subnet group; the module must not create one."
  }

  assert {
    condition     = length(aws_db_subnet_group.aurora) == 0
    error_message = "A standard instance must not create the Aurora subnet group."
  }
}

# The bug: an own-VPC standard instance had no subnet group, so RDS put it in
# the default VPC while its security group was in the env VPC.
run "own_vpc_standard_uses_env_public_subnets" {
  command = plan

  variables {
    default_vpc = false
  }

  assert {
    condition     = length(aws_db_subnet_group.database) == 1
    error_message = "An own-VPC standard instance needs its own DB subnet group in the env VPC."
  }

  assert {
    condition     = aws_db_subnet_group.database[0].name == "myapp-postgres-subnet-dev"
    error_message = "The standard subnet group must be named <project>-postgres-subnet-<env>."
  }

  assert {
    condition     = toset(aws_db_subnet_group.database[0].subnet_ids) == toset(["subnet-aaa00000000000001", "subnet-aaa00000000000002"])
    error_message = "The standard subnet group must use var.subnet_ids, the env VPC's public subnets."
  }

  assert {
    condition     = aws_db_instance.database[0].db_subnet_group_name == "myapp-postgres-subnet-dev"
    error_message = "The standard instance must be placed in the module's subnet group, not the account's default one."
  }
}

# An Aurora cluster deployed before this change must see no plan change: same
# group name, same subnets.
run "existing_aurora_public_keeps_its_subnet_group" {
  command = plan

  variables {
    aurora = true
  }

  assert {
    condition     = aws_db_subnet_group.aurora[0].name == "myapp-aurora-subnet-dev"
    error_message = "The Aurora subnet group must keep its deployed name <project>-aurora-subnet-<env>."
  }

  assert {
    condition     = toset(aws_db_subnet_group.aurora[0].subnet_ids) == toset(["subnet-aaa00000000000001", "subnet-aaa00000000000002"])
    error_message = "The public Aurora subnet group must keep var.subnet_ids."
  }

  assert {
    condition     = aws_rds_cluster.aurora[0].db_subnet_group_name == "myapp-aurora-subnet-dev"
    error_message = "The Aurora cluster must use the Aurora subnet group."
  }

  assert {
    condition     = length(aws_db_subnet_group.database) == 0
    error_message = "Aurora must not create the standard instance's subnet group."
  }
}

run "private_standard_uses_private_subnets" {
  command = plan

  variables {
    private_subnets    = true
    private_subnet_ids = ["subnet-bbb00000000000001", "subnet-bbb00000000000002"]
  }

  assert {
    condition     = aws_db_subnet_group.database[0].name == "myapp-postgres-private-subnet-dev"
    error_message = "A private standard instance's subnet group must be named <project>-postgres-private-subnet-<env>."
  }

  assert {
    condition     = toset(aws_db_subnet_group.database[0].subnet_ids) == toset(["subnet-bbb00000000000001", "subnet-bbb00000000000002"])
    error_message = "A private standard instance's subnet group must use private_subnet_ids."
  }

  assert {
    condition     = aws_db_instance.database[0].db_subnet_group_name == "myapp-postgres-private-subnet-dev"
    error_message = "The private standard instance must use the private subnet group."
  }
}

run "private_aurora_uses_private_subnets" {
  command = plan

  variables {
    aurora             = true
    private_subnets    = true
    private_subnet_ids = ["subnet-bbb00000000000001", "subnet-bbb00000000000002"]
  }

  assert {
    condition     = aws_db_subnet_group.aurora[0].name == "myapp-aurora-private-subnet-dev"
    error_message = "A private Aurora subnet group must be named <project>-aurora-private-subnet-<env>."
  }

  assert {
    condition     = toset(aws_db_subnet_group.aurora[0].subnet_ids) == toset(["subnet-bbb00000000000001", "subnet-bbb00000000000002"])
    error_message = "A private Aurora subnet group must use private_subnet_ids."
  }

  assert {
    condition     = aws_rds_cluster.aurora[0].db_subnet_group_name == "myapp-aurora-private-subnet-dev"
    error_message = "The private Aurora cluster must use the private subnet group."
  }
}

# ---------------------------------------------------------------------------
# An existing database never moves
# ---------------------------------------------------------------------------

# Standard, public -> private.
run "flip_standard_apply_public" {
  command = apply

  assert {
    condition     = aws_db_instance.database[0].db_subnet_group_name == "myapp-postgres-subnet-dev"
    error_message = "The applied public instance must be in the public subnet group."
  }
}

run "flip_standard_public_to_private_is_refused" {
  command = plan

  variables {
    private_subnets    = true
    private_subnet_ids = ["subnet-bbb00000000000001", "subnet-bbb00000000000002"]
  }

  expect_failures = [aws_db_instance.database]
}

# Own VPC -> default VPC. The plan would delete the subnet group the database
# still uses.
run "flip_standard_own_vpc_to_default_vpc_is_refused" {
  command = plan

  variables {
    default_vpc = true
  }

  expect_failures = [aws_db_instance.database]
}

# The original values again: the refused plans changed nothing. The id is known
# at plan only when the instance is kept.
run "flip_standard_back_to_own_vpc_keeps_the_instance" {
  command = plan

  assert {
    condition     = length(aws_db_instance.database[0].id) > 0
    error_message = "The original values must keep the existing instance."
  }

  assert {
    condition     = aws_db_instance.database[0].db_subnet_group_name == "myapp-postgres-subnet-dev"
    error_message = "The existing instance must stay in its subnet group."
  }
}

# Aurora, public -> private. The apply destroys the standard instance above and
# creates a fresh cluster.
run "flip_aurora_apply_public" {
  command = apply

  variables {
    aurora = true
  }

  assert {
    condition     = aws_rds_cluster.aurora[0].db_subnet_group_name == "myapp-aurora-subnet-dev"
    error_message = "The applied public cluster must be in the public Aurora subnet group."
  }
}

run "flip_aurora_public_to_private_is_refused" {
  command = plan

  variables {
    aurora             = true
    private_subnets    = true
    private_subnet_ids = ["subnet-bbb00000000000001", "subnet-bbb00000000000002"]
  }

  expect_failures = [aws_rds_cluster.aurora]
}

# Standard, private -> public (the key removed). The apply destroys the cluster
# above and creates a fresh private instance.
run "flip_standard_apply_private" {
  command = apply

  variables {
    private_subnets    = true
    private_subnet_ids = ["subnet-bbb00000000000001", "subnet-bbb00000000000002"]
  }

  assert {
    condition     = aws_db_instance.database[0].db_subnet_group_name == "myapp-postgres-private-subnet-dev"
    error_message = "The applied private instance must be in the private subnet group."
  }
}

run "flip_standard_private_to_public_is_refused" {
  command = plan

  expect_failures = [aws_db_instance.database]
}

run "flip_standard_private_to_default_vpc_is_refused" {
  command = plan

  variables {
    default_vpc = true
  }

  expect_failures = [aws_db_instance.database]
}

# An existing default-VPC instance keeps planning. The Aurora apply only clears
# the private instance above, so that the default-VPC one is created fresh.
run "default_vpc_clear_state" {
  command = apply

  variables {
    aurora = true
  }
}

run "default_vpc_apply_standard" {
  command = apply

  variables {
    default_vpc = true
  }
}

run "default_vpc_existing_standard_still_plans" {
  command = plan

  variables {
    default_vpc = true
  }

  assert {
    condition     = length(aws_db_instance.database[0].id) > 0
    error_message = "An existing default-VPC instance must be kept."
  }
}

# ---------------------------------------------------------------------------
# Guards
# ---------------------------------------------------------------------------

run "private_with_no_private_subnets_is_refused" {
  command = plan

  variables {
    private_subnets    = true
    private_subnet_ids = []
  }

  expect_failures = [aws_db_subnet_group.database]
}

run "private_in_default_vpc_is_refused" {
  command = plan

  variables {
    aurora             = true
    default_vpc        = true
    private_subnets    = true
    private_subnet_ids = ["subnet-bbb00000000000001", "subnet-bbb00000000000002"]
  }

  expect_failures = [aws_db_subnet_group.aurora]
}

# No subnet group exists in this shape, so the guard is on the instance. The
# instance in state is the default-VPC one above, which the placement
# postcondition accepts, so only the guard can fail this plan.
run "private_standard_in_default_vpc_is_refused" {
  command = plan

  variables {
    default_vpc        = true
    private_subnets    = true
    private_subnet_ids = ["subnet-bbb00000000000001", "subnet-bbb00000000000002"]
  }

  expect_failures = [aws_db_instance.database]
}
