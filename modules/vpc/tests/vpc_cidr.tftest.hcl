# REQ-7/R1.7 and REQ-17/M3: Go range policy MUST NOT be copied here.
# The module accepts IPv4 network addresses /16 through /20, including ranges
# that meroku would reject for a new environment. It must not lock out an
# existing deployment during terraform plan.
#
# Validation needs Terraform >= 1.2.6; mock_provider tests need >= 1.7.
# The supplied contract says CI uses 1.9.8.
# Run after copying: terraform -chdir=modules/vpc test
#
# Only data-source TYPES are in the contract, not instance labels. A type-level
# mock supplies two AZs without guessing an override_data target address.
# Every run is a plan with a mock provider: no AWS calls and no infrastructure.

mock_provider "aws" {
  mock_data "aws_availability_zones" {
    defaults = {
      names    = ["us-east-1a", "us-east-1b"]
      zone_ids = ["synthetic-az-one", "synthetic-az-two"]
    }
  }

  mock_data "aws_region" {
    defaults = {
      region = "us-east-1"
    }
  }
}

variables {
  project         = "qa-project"
  env             = "qa"
  egress_strategy = "public_ip"
}

# TEST-40; REQ-7/R1.7, REQ-17/M3.
run "default_range" {
  command = plan

  assert {
    condition     = var.vpc_cidr == "10.0.0.0/16"
    error_message = "An unset range must retain the legacy Terraform default; only new YAML creation randomizes."
  }
}

run "accepts_16" {
  command = plan
  variables {
    vpc_cidr = "10.42.0.0/16"
  }
}

run "accepts_17" {
  command = plan
  variables {
    vpc_cidr = "10.42.128.0/17"
  }
}

run "accepts_18" {
  command = plan
  variables {
    vpc_cidr = "10.42.192.0/18"
  }
}

run "accepts_19" {
  command = plan
  variables {
    vpc_cidr = "10.42.224.0/19"
  }
}

run "accepts_20" {
  command = plan
  variables {
    vpc_cidr = "10.42.240.0/20"
  }
}

# TEST-41; REQ-1/R1.1, REQ-7/R1.7, REQ-17/M3.
run "rejects_text_format" {
  command = plan
  variables {
    vpc_cidr = "not-a-network"
  }
  expect_failures = [var.vpc_cidr]
}

run "rejects_missing_size_format" {
  command = plan
  variables {
    vpc_cidr = "10.42.0.0"
  }
  expect_failures = [var.vpc_cidr]
}

run "rejects_ipv6_format" {
  command = plan
  variables {
    vpc_cidr = "fd00::/16"
  }
  expect_failures = [var.vpc_cidr]
}

run "rejects_invalid_octet_format" {
  command = plan
  variables {
    vpc_cidr = "999.42.0.0/16"
  }
  expect_failures = [var.vpc_cidr]
}

# TEST-42; REQ-1/R1.1, REQ-7/R1.7, REQ-17/M3.
run "rejects_host_bits_16" {
  command = plan
  variables {
    vpc_cidr = "10.42.1.0/16"
  }
  expect_failures = [var.vpc_cidr]
}

run "rejects_host_bits_20" {
  command = plan
  variables {
    vpc_cidr = "10.42.240.1/20"
  }
  expect_failures = [var.vpc_cidr]
}

# TEST-43; REQ-7/R1.7, REQ-17/M3.
run "rejects_15" {
  command = plan
  variables {
    vpc_cidr = "10.42.0.0/15"
  }
  expect_failures = [var.vpc_cidr]
}

run "rejects_21" {
  command = plan
  variables {
    vpc_cidr = "10.42.0.0/21"
  }
  expect_failures = [var.vpc_cidr]
}

# TEST-44; REQ-7/R1.7, REQ-17/M3.
# These plans prove range policy stays in Go, not Terraform validation.
run "preserves_docker_private_range" {
  command = plan
  variables {
    vpc_cidr = "172.16.0.0/16"
  }
}

run "preserves_aws_default_range" {
  command = plan
  variables {
    vpc_cidr = "172.31.0.0/16"
  }
}

run "preserves_link_local_range" {
  command = plan
  variables {
    vpc_cidr = "169.254.0.0/16"
  }
}

run "preserves_restricted_range" {
  command = plan
  variables {
    vpc_cidr = "198.19.0.0/16"
  }
}

run "preserves_public_range" {
  command = plan
  variables {
    vpc_cidr = "8.8.0.0/16"
  }
}

run "preserves_192_private_range" {
  command = plan
  variables {
    vpc_cidr = "192.168.224.0/19"
  }
}

# TEST-45; REQ-7/R1.7, REQ-17/M3.
# accepts_20 above covers public_ip. These cover subnet indices 10 and 11 too.
run "accepts_20_nat" {
  command = plan
  variables {
    vpc_cidr        = "10.42.240.0/20"
    egress_strategy = "nat_gateway"
  }
}

run "accepts_20_nat_ha" {
  command = plan
  variables {
    vpc_cidr        = "10.42.240.0/20"
    egress_strategy = "nat_gateway_ha"
  }
}
