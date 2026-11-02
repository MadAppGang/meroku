# services[].network_access (schema v30): who may open a connection to a
# service's container port. See local.service_network_access in services.tf for
# the table this file pins.
#
# Every run is `command = plan`; nothing here needs an applied state. The ALB
# run is LAST: it touches the most unrelated resources (listener, Route53 alias,
# the aws_lb data source), and a run that ERRORS skips every run after it.
#
# The services are the same in every run so the for_each keys mean the same
# thing throughout:
#
#   legacy  — full_open. What every pre-v30 service is pinned to by the
#             migration; it must produce exactly the pre-v30 resources.
#   web     — public, stated.
#   implied — no network_access at all, which must mean public.
#   bridge  — internal. The case the feature was asked for.
#   worker  — none.
#
# Run: terraform test  (from modules/workloads)

mock_provider "aws" {
  # Same defaults as tests/env_secret_collision.tftest.hcl, for the same reason:
  # the provider validates policy JSON, CIDRs and ARNs client-side. Account
  # 000000000000 is synthetic; this repository is public.
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }

  mock_data "aws_vpc" {
    defaults = {
      cidr_block = "10.0.0.0/16"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::000000000000:role/mock"
    }
  }
  mock_resource "aws_iam_policy" {
    defaults = {
      arn = "arn:aws:iam::000000000000:policy/mock"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock"
    }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = {
      arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock"
    }
  }
  mock_resource "aws_lambda_function" {
    defaults = {
      arn = "arn:aws:lambda:us-east-1:000000000000:function:mock"
    }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = {
      arn = "arn:aws:events:us-east-1:000000000000:rule/mock"
    }
  }
}

mock_provider "null" {}
mock_provider "random" {}

variables {
  project          = "acme"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:acme/acme:ref:refs/heads/main"]

  services = [
    { name = "legacy", network_access = "full_open", docker_image = "public.ecr.aws/nginx/nginx:stable" },
    { name = "web", network_access = "public", docker_image = "public.ecr.aws/nginx/nginx:stable" },
    { name = "implied", docker_image = "public.ecr.aws/nginx/nginx:stable" },
    {
      name                 = "bridge"
      network_access       = "internal"
      container_port       = 8080
      host_port            = 8080
      docker_image         = "public.ecr.aws/nginx/nginx:stable"
      health_check_command = ["CMD-SHELL", "wget -qO- http://localhost:8080/health/live || exit 1"]
    },
    { name = "worker", network_access = "none", docker_image = "public.ecr.aws/nginx/nginx:stable" },
  ]
}

# ---------------------------------------------------------------------------
# A project on the default API Gateway path.
# ---------------------------------------------------------------------------
run "api_gateway_project_routes_public_services_through_the_vpc_link" {
  command = plan

  variables {
    enable_alb = false
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = length(aws_lb_target_group.services) == 0 && length(aws_lb_listener_rule.services) == 0
    error_message = "With the ALB off there are no target groups or listener rules for any mode."
  }

  assert {
    condition     = sort(keys(aws_apigatewayv2_route.services)) == tolist(["implied", "legacy", "web"])
    error_message = "With the ALB off, public and full_open services are routed by API Gateway, internal and none are not. Got ${jsonencode(sort(keys(aws_apigatewayv2_route.services)))}."
  }

  # The VPC Link carries the service's own security group, so `self` is what
  # admits it — and nothing else.
  assert {
    condition     = jsonencode([for r in aws_security_group.services["web"].ingress : [r.self, r.cidr_blocks, length(r.security_groups)]]) == jsonencode([[true, [], 0]])
    error_message = "public behind API Gateway must admit only its own security group (the VPC Link). Got ${jsonencode(aws_security_group.services["web"].ingress)}."
  }

  # Known and empty at plan, not "(not yet known)". Zero ingress BLOCKS would
  # leave the attribute unmanaged and any existing rule in place; the attribute
  # form set to [] is what makes none remove 0.0.0.0/0 from a migrated service.
  assert {
    condition     = length(aws_security_group.services["worker"].ingress) == 0
    error_message = "none must plan an empty, managed ingress set."
  }
}

# ---------------------------------------------------------------------------
# The opt-in container health check.
# ---------------------------------------------------------------------------
run "health_check_is_rendered_only_when_a_command_is_given" {
  command = plan

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = jsonencode(local.service_health_check["bridge"].healthCheck.command) == jsonencode(["CMD-SHELL", "wget -qO- http://localhost:8080/health/live || exit 1"])
    error_message = "bridge sets health_check_command, so its container must get a healthCheck with that command."
  }

  # No key at all — not an empty healthCheck. An added key changes the container
  # JSON, which is a new task-definition revision in every environment.
  assert {
    condition     = alltrue([for k in ["legacy", "web", "implied", "worker"] : length(local.service_health_check[k]) == 0])
    error_message = "A service without health_check_command must contribute no healthCheck key."
  }
}

run "an_unknown_network_access_value_is_refused" {
  command = plan

  variables {
    services = [{ name = "typo", network_access = "private" }]
  }

  expect_failures = [var.services]
}

# ---------------------------------------------------------------------------
# A project with the ALB on.
# ---------------------------------------------------------------------------
run "alb_project_wires_only_front_door_services_to_the_alb" {
  command = plan

  variables {
    enable_alb                 = true
    alb_arn                    = "arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/mock/0000000000000000"
    alb_security_group_id      = "sg-0a1b0000000000000"
    subdomains_certificate_arn = "arn:aws:acm:us-east-1:000000000000:certificate/00000000-0000-0000-0000-000000000000"
    domain_zone_id             = "Z00000000000000000000"
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  # REGRESSION: MTL-359 — old ECS task never stopped after a service left the ALB — Fixed in /dev:fix session dev-fix-20261102-115054-mtl359
  # The target group must not follow network_access. If moving a service from
  # public/full_open to internal/none deletes its target group, Terraform deletes
  # it in the same apply before the UpdateService that drops load_balancer, and
  # ECS never stops the old task. The listener-rule and load_balancer asserts
  # below are what keep internal and none services off the ALB.
  # Every fixture service is Fargate/awsvpc ("bridge" is only a name; it has no
  # runtime = "ec2"), so all five keys belong in services, and none in
  # services_instance.
  assert {
    condition     = sort(keys(aws_lb_target_group.services)) == tolist(["bridge", "implied", "legacy", "web", "worker"]) && length(aws_lb_target_group.services_instance) == 0
    error_message = "With the ALB on, every awsvpc service needs an ALB target group whatever its network_access. A target group deleted in the same apply as the service leaves the ALB strands the old ECS task (MTL-359). Got ${jsonencode(sort(keys(aws_lb_target_group.services)))}, want [\"bridge\",\"implied\",\"legacy\",\"web\",\"worker\"]."
  }

  assert {
    condition     = sort(keys(aws_lb_listener_rule.services)) == tolist(["implied", "legacy", "web"])
    error_message = "Only public and full_open services get a listener rule. An internal service with a rule is reachable from the internet through the ALB. Got ${jsonencode(sort(keys(aws_lb_listener_rule.services)))}."
  }

  # Priorities index every service name, so dropping bridge must not renumber
  # the rules after it: sorted names are bridge, implied, legacy, web, worker.
  assert {
    condition     = aws_lb_listener_rule.services["web"].priority == 13
    error_message = "web's listener rule priority must stay 13 (its index among ALL services + 10). Got ${aws_lb_listener_rule.services["web"].priority}."
  }

  assert {
    condition     = length(aws_ecs_service.services["web"].load_balancer) == 1 && length(aws_ecs_service.services["bridge"].load_balancer) == 0 && length(aws_ecs_service.services["worker"].load_balancer) == 0
    error_message = "Only services on the ALB may carry a load_balancer block."
  }

  # In an ALB project the API Gateway is not public's front door, so only the
  # full_open service keeps its route, exactly as before v30.
  assert {
    condition     = jsonencode([keys(aws_apigatewayv2_route.services), keys(aws_apigatewayv2_vpc_link.services), keys(aws_apigatewayv2_integration.services)]) == jsonencode([["legacy"], ["legacy"], ["legacy"]])
    error_message = "With the ALB on, only full_open services keep an API Gateway route, integration and VPC Link. Got routes ${jsonencode(keys(aws_apigatewayv2_route.services))}."
  }

  # full_open: the pre-v30 rule, unchanged.
  assert {
    condition     = jsonencode([for r in aws_security_group.services["legacy"].ingress : [r.cidr_blocks, r.ipv6_cidr_blocks, r.from_port, r.description]]) == jsonencode([[["0.0.0.0/0"], ["::/0"], 3000, ""]])
    error_message = "full_open must keep the single pre-v30 ingress rule byte for byte. Got ${jsonencode(aws_security_group.services["legacy"].ingress)}."
  }

  assert {
    condition = alltrue([for k in ["web", "implied"] :
      jsonencode([for r in aws_security_group.services[k].ingress : [sort(r.security_groups), r.cidr_blocks, r.self]]) == jsonencode([[["sg-0a1b0000000000000"], [], false]])
    ])
    error_message = "public behind the ALB must admit the ALB's security group and no CIDR. Got ${jsonencode(aws_security_group.services["web"].ingress)}."
  }

  assert {
    condition     = jsonencode([for r in aws_security_group.services["bridge"].ingress : [r.cidr_blocks, r.ipv6_cidr_blocks, r.from_port]]) == jsonencode([[["10.0.0.0/16"], [], 8080]])
    error_message = "internal must admit the VPC CIDR only, on the container port. Got ${jsonencode(aws_security_group.services["bridge"].ingress)}."
  }

  assert {
    condition     = length(aws_security_group.services["worker"].ingress) == 0
    error_message = "none must have no ingress rule at all. Got ${jsonencode(aws_security_group.services["worker"].ingress)}."
  }

  # Outbound and the public IP are what pull the image from ECR; no mode may
  # touch them.
  assert {
    condition = alltrue([for k, sg in aws_security_group.services :
      jsonencode([for r in sg.egress : r.cidr_blocks]) == jsonencode([["0.0.0.0/0"]])
    ])
    error_message = "Every network_access mode must keep the open egress rule."
  }

  assert {
    condition = alltrue([for k, svc in aws_ecs_service.services :
      svc.network_configuration[0].assign_public_ip == true
    ])
    error_message = "network_access must never change assign_public_ip."
  }
}

# ---------------------------------------------------------------------------
# An ALB project whose services run in bridge mode on an EC2 pool.
#
# The run above pins the Fargate half of MTL-359: every fixture service is
# awsvpc, so aws_lb_target_group.services_instance is never planned there. This
# run replaces the service list with two services on one bridge-mode pool, the
# only thing that routes a service to the "instance" target group. The shared
# list stays as it is: the run above indexes all its names for priorities.
# ---------------------------------------------------------------------------
run "alb_project_keeps_bridge_mode_target_groups_off_the_alb" {
  command = plan

  variables {
    enable_alb                 = true
    alb_arn                    = "arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/mock/0000000000000000"
    alb_security_group_id      = "sg-0a1b0000000000000"
    subdomains_certificate_arn = "arn:aws:acm:us-east-1:000000000000:certificate/00000000-0000-0000-0000-000000000000"
    domain_zone_id             = "Z00000000000000000000"

    compute_pools = [
      { name = "general", instance_types = ["t3.small"], network_mode = "bridge" },
    ]

    services = [
      { name = "front", network_access = "full_open", runtime = "ec2", compute_pool = "general", docker_image = "public.ecr.aws/nginx/nginx:stable" },
      { name = "back", network_access = "internal", runtime = "ec2", compute_pool = "general", docker_image = "public.ecr.aws/nginx/nginx:stable" },
    ]
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  # Guards the premise: with neither service in bridge mode, the asserts below
  # would be checking an empty services_instance for the wrong reason.
  assert {
    condition     = local.service_bridge["front"] && local.service_bridge["back"]
    error_message = "Both services must be bridge-mode for this run to test services_instance. Got ${jsonencode(local.service_network_modes)}."
  }

  # REGRESSION: MTL-359 — bridge-mode target group must outlive the service's time on the ALB
  # Same failure as the Fargate case, on the "instance" twin: if moving a
  # bridge-mode service to internal or none deletes its target group, Terraform
  # deletes it in the same apply before the UpdateService that drops
  # load_balancer, and ECS never stops the old task.
  assert {
    condition     = sort(keys(aws_lb_target_group.services_instance)) == tolist(["back", "front"])
    error_message = "With the ALB on, every bridge-mode service needs an instance target group whatever its network_access. A target group deleted in the same apply as the service leaves the ALB strands the old ECS task (MTL-359). Got ${jsonencode(sort(keys(aws_lb_target_group.services_instance)))}, want [\"back\",\"front\"]."
  }

  assert {
    condition     = length(aws_lb_target_group.services) == 0
    error_message = "A bridge-mode service must get the \"instance\" target group, never the \"ip\" one: target_type is ForceNew. Got ${jsonencode(sort(keys(aws_lb_target_group.services)))} in aws_lb_target_group.services."
  }

  assert {
    condition     = sort(keys(aws_lb_listener_rule.services)) == tolist(["front"])
    error_message = "Only the full_open service gets a listener rule. An internal service with a rule is reachable from the internet through the ALB. Got ${jsonencode(sort(keys(aws_lb_listener_rule.services)))}."
  }

  assert {
    condition     = length(aws_ecs_service.services["front"].load_balancer) == 1 && length(aws_ecs_service.services["back"].load_balancer) == 0
    error_message = "Only the full_open service may carry a load_balancer block. The internal one keeps its target group so the old task can be deregistered, but must not be registered in it."
  }
}
