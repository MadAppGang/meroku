# Effective SG access, serverless cap and TLS connection variables.
mock_provider "aws" {
  mock_resource "aws_elasticache_serverless_cache" {
    defaults = {
      # Computed endpoint attributes are lists of objects in the provider schema.
      endpoint = [{ address = "cache.example.com", port = 6379 }]
    }
  }
}

override_resource {
  target = aws_security_group.valkey
  values = { id = "sg-00000000000000999" }
}

variables {
  project    = "myapp"
  env        = "dev"
  vpc_id     = "vpc-00000000000000000"
  subnet_ids = ["subnet-00000000000000001", "subnet-00000000000000002"]
  workloads = {
    api    = { sg_key = "workload:api", security_group_id = "sg-00000000000000001" }
    worker = { sg_key = "workload:worker", security_group_id = "sg-00000000000000002" }
  }
  access = ["api"]
}

run "only_listed_workloads_get_both_tls_ports" {
  command = plan

  assert {
    condition = (
      length(aws_vpc_security_group_ingress_rule.access) == 2 &&
      toset(output.ingress_keys) == toset(["workload:api"]) &&
      toset([for r in aws_vpc_security_group_ingress_rule.access : "${r.from_port}:${r.to_port}:${r.ip_protocol}:${r.referenced_security_group_id}"]) ==
      toset(["6379:6379:tcp:sg-00000000000000001", "6380:6380:tcp:sg-00000000000000001"]) &&
      length(output.environment["worker"]) == 0
    )
    error_message = "Only a listed workload's effective SG may reach the cache, on exactly TCP 6379 and 6380; unlisted workloads get no connection variables."
  }
}

run "bridge_workloads_share_one_rule_pair" {
  command = plan

  variables {
    workloads = {
      backend = { sg_key = "ecs_instance", security_group_id = "sg-00000000000000003" }
      api     = { sg_key = "ecs_instance", security_group_id = "sg-00000000000000003" }
    }
    access = ["backend", "api"]
  }

  assert {
    condition = (
      toset(output.ingress_keys) == toset(["ecs_instance"]) &&
      length(aws_vpc_security_group_ingress_rule.access) == 2 &&
      toset([for r in aws_vpc_security_group_ingress_rule.access : "${r.from_port}:${r.to_port}:${r.referenced_security_group_id}"]) ==
      toset(["6379:6379:sg-00000000000000003", "6380:6380:sg-00000000000000003"])
    )
    error_message = "A bridge backend and bridge service sharing an instance SG must produce one rule pair, not duplicate ingress."
  }
}

run "fargate_marker_name_does_not_share_the_bridge_key" {
  command = plan

  variables {
    workloads = {
      backend       = { sg_key = "ecs_instance", security_group_id = "sg-00000000000000003" }
      _ecs_instance = { sg_key = "workload:_ecs_instance", security_group_id = "sg-00000000000000004" }
    }
    access = ["backend", "_ecs_instance"]
  }

  assert {
    condition = (
      toset(output.ingress_keys) == toset(["ecs_instance", "workload:_ecs_instance"]) &&
      length(aws_vpc_security_group_ingress_rule.access) == 4 &&
      toset([for r in aws_vpc_security_group_ingress_rule.access : "${r.from_port}:${r.referenced_security_group_id}"]) == toset([
        "6379:sg-00000000000000003", "6380:sg-00000000000000003",
        "6379:sg-00000000000000004", "6380:sg-00000000000000004"
      ])
    )
    error_message = "A Fargate workload named _ecs_instance must keep its own SG pair beside a bridge workload."
  }
}

run "serverless_limits_name_and_tls_variables_are_explicit" {
  # The endpoint and target SG ID are apply-time computed values.
  command = apply

  variables {
    project        = "My_App_With_An_Overlength_Serverless_Cache_Project_Name"
    engine_version = "8"
    max_storage_gb = 3
  }

  assert {
    condition = (
      aws_elasticache_serverless_cache.valkey.engine == "valkey" &&
      aws_elasticache_serverless_cache.valkey.major_engine_version == "8" &&
      aws_elasticache_serverless_cache.valkey.connection_type == "vpc" &&
      length(aws_elasticache_serverless_cache.valkey.name) <= 40 &&
      can(regex("^[a-z][a-z0-9-]*[a-z0-9]$", aws_elasticache_serverless_cache.valkey.name)) &&
      !strcontains(aws_elasticache_serverless_cache.valkey.name, "_")
    )
    error_message = "The VPC-only serverless Valkey name must be lower-case, hyphenated and at most forty characters even when the project contains underscores."
  }

  assert {
    condition = (
      one(one(aws_elasticache_serverless_cache.valkey.cache_usage_limits).data_storage).maximum == 3 &&
      one(one(aws_elasticache_serverless_cache.valkey.cache_usage_limits).data_storage).unit == "GB" &&
      toset(aws_elasticache_serverless_cache.valkey.subnet_ids) == toset(var.subnet_ids) &&
      toset(aws_elasticache_serverless_cache.valkey.security_group_ids) == toset(["sg-00000000000000999"]) &&
      alltrue([for r in aws_vpc_security_group_ingress_rule.access : r.security_group_id == "sg-00000000000000999"])
    )
    error_message = "Storage must keep the requested GB maximum; the cache and access rules must use its own VPC SG and subnets."
  }

  assert {
    condition = (
      output.address == "cache.example.com" && output.port == 6379 &&
      output.security_group_id == "sg-00000000000000999" &&
      { for e in output.environment["api"] : e.name => e.value } == {
        VALKEY_HOST = "cache.example.com"
        VALKEY_PORT = "6379"
        VALKEY_URL  = "rediss://cache.example.com:6379"
      } &&
      length(output.environment["worker"]) == 0
    )
    error_message = "Listed workloads must receive exactly the endpoint host, string port and rediss URL, without a password or variables for unlisted workloads."
  }
}

run "empty_access_opens_no_ingress" {
  command = plan

  variables {
    access = []
  }

  assert {
    condition = (
      length(aws_vpc_security_group_ingress_rule.access) == 0 &&
      length(output.ingress_keys) == 0 &&
      alltrue([for entries in output.environment : length(entries) == 0])
    )
    error_message = "An enabled cache without an access list must open no ingress and inject no connection variables."
  }
}
