# The optional ALB settings: alb_ssl_policy, alb_deregistration_delay,
# backend_health_check_grace_period and remove_api_gateway, and the single
# api_domain record shared by both ingress modes.
#
# Every new variable defaults to what the module did before it existed, so an
# environment that sets none of them must plan exactly the old resources.
#
# Ordering matters:
#   - apply runs build the state that later plan runs read (run 1 builds an
#     API-Gateway-mode state, run 7 an ALB-mode state);
#   - an `X.id != ""` assert in a plan run fails with "Unknown condition value"
#     when X is planned as a create, so it proves X keeps its address and is
#     updated in place. Under mock_provider a provider-level ForceNew does not
#     surface, so these asserts prove address stability, not provider behaviour;
#   - runs that are expected to fail validation come last, because a run that
#     errors skips every run after it.
#
# The two services are the same in every run: `legacy` is full_open (always
# routed by API Gateway), `web` is public (API Gateway only while the ALB is
# off). `legacy` is what proves remove_api_gateway stays valid at module level
# even when the Go refusal of full_open is bypassed.
#
# Account 000000000000 and every id below are synthetic; this repository is
# public. The CI Terraform is 1.9.8, so no state_key and no override_during.
#
# Run: terraform test -filter=tests/alb_settings.tftest.hcl  (from modules/workloads)

mock_provider "aws" {
  # The provider validates policy JSON, CIDRs and ARNs client-side.
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

  mock_data "aws_lb" {
    defaults = {
      dns_name = "mock-alb-0000000000.us-east-1.elb.amazonaws.com"
      zone_id  = "ZMOCKALB000000"
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
  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:us-east-1:000000000000:targetgroup/mock/0000000000000000"
    }
  }
  mock_resource "aws_lb_listener" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:us-east-1:000000000000:listener/app/mock/0000000000000000/0000000000000000"
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

  enable_custom_domain       = true
  create_api_domain_record   = true
  api_certificate_arn        = "arn:aws:acm:us-east-1:000000000000:certificate/00000000-0000-0000-0000-000000000000"
  subdomains_certificate_arn = "arn:aws:acm:us-east-1:000000000000:certificate/00000000-0000-0000-0000-000000000000"
  domain_zone_id             = "Z00000000000000000000"
  alb_arn                    = "arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/mock/0000000000000000"
  alb_security_group_id      = "sg-0a1b0000000000000"

  services = [
    { name = "legacy", network_access = "full_open", docker_image = "public.ecr.aws/nginx/nginx:stable" },
    { name = "web", network_access = "public", docker_image = "public.ecr.aws/nginx/nginx:stable" },
  ]
}

# ---------------------------------------------------------------------------
# enable_alb = false, remove_api_gateway = false (defaults). Builds the
# API-Gateway-mode state the next runs plan against.
# ---------------------------------------------------------------------------
run "alb_off_defaults_keep_the_gateway" {
  command = apply

  variables {
    enable_alb = false
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = alltrue([
      length(aws_apigatewayv2_api.api_gateway) == 1,
      length(aws_apigatewayv2_vpc_link.backend) == 1,
      length(aws_apigatewayv2_stage.backend) == 1,
      length(aws_cloudwatch_log_group.api_gateway_logs) == 1,
      length(aws_apigatewayv2_integration.backend) == 1,
      length(aws_apigatewayv2_route.backend) == 1,
    ])
    error_message = "With the ALB off the API Gateway API, backend VPC Link, stage, access-log group, backend integration and backend route must each exist once."
  }

  assert {
    condition     = output.api_gateway_endpoint != "" && output.api_gateway_id != ""
    error_message = "With the gateway created, api_gateway_endpoint and api_gateway_id must not be empty. Got endpoint=${output.api_gateway_endpoint} id=${output.api_gateway_id}."
  }

  assert {
    condition     = aws_route53_record.backend[0].alias[0].name == aws_apigatewayv2_domain_name.backend[0].domain_name_configuration[0].target_domain_name
    error_message = "With the ALB off the api_domain record must alias the API Gateway custom domain. Got ${aws_route53_record.backend[0].alias[0].name}."
  }

  assert {
    condition     = aws_ecs_service.backend.health_check_grace_period_seconds == null
    error_message = "With the grace period unset and the ALB off, health_check_grace_period_seconds must be null."
  }

  assert {
    condition     = length(aws_ecs_service.backend.load_balancer) == 0
    error_message = "With the ALB off the backend service must have no load_balancer block."
  }

  assert {
    condition     = length(aws_lb_listener.https) == 0
    error_message = "With the ALB off there must be no HTTPS listener."
  }
}

# ---------------------------------------------------------------------------
# enable_alb = false, remove_api_gateway = true: remove is ignored, because
# the gateway is then the only ingress.
# ---------------------------------------------------------------------------
run "alb_off_ignores_remove_api_gateway" {
  command = plan

  variables {
    enable_alb         = false
    remove_api_gateway = true
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = alltrue([
      length(aws_apigatewayv2_api.api_gateway) == 1,
      length(aws_apigatewayv2_vpc_link.backend) == 1,
      length(aws_apigatewayv2_stage.backend) == 1,
      length(aws_cloudwatch_log_group.api_gateway_logs) == 1,
      length(aws_apigatewayv2_integration.backend) == 1,
      length(aws_apigatewayv2_route.backend) == 1,
    ])
    error_message = "remove_api_gateway must be ignored while the ALB is off: all six gateway resources must still exist."
  }

  # Known ids: the resources keep the addresses run 1 applied, none is a create.
  assert {
    condition = alltrue([
      aws_apigatewayv2_api.api_gateway[0].id != "",
      aws_apigatewayv2_vpc_link.backend[0].id != "",
      aws_apigatewayv2_stage.backend[0].id != "",
      aws_cloudwatch_log_group.api_gateway_logs[0].id != "",
      aws_apigatewayv2_integration.backend[0].id != "",
      aws_apigatewayv2_route.backend[0].id != "",
    ])
    error_message = "The six gateway resources must keep their applied addresses (known ids), not be planned as creates."
  }

  assert {
    condition     = sort(keys(aws_apigatewayv2_route.services)) == tolist(["legacy", "web"])
    error_message = "With the ALB off the public and full_open services must keep their API Gateway routes. Got ${jsonencode(sort(keys(aws_apigatewayv2_route.services)))}."
  }

  assert {
    condition     = output.api_gateway_endpoint != "" && output.api_gateway_id != ""
    error_message = "With the ALB off the gateway outputs must not be empty."
  }
}

# ---------------------------------------------------------------------------
# enable_alb = true, remove_api_gateway = false, every new variable at its
# default. Planned against run 1's API-Gateway-mode state: the switch.
# ---------------------------------------------------------------------------
run "alb_on_with_defaults" {
  command = plan

  variables {
    enable_alb = true
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = aws_lb_listener.https[0].ssl_policy == "ELBSecurityPolicy-2016-08"
    error_message = "The default TLS policy must stay ELBSecurityPolicy-2016-08. Got ${aws_lb_listener.https[0].ssl_policy}."
  }

  assert {
    condition = alltrue([
      tostring(aws_lb_target_group.backend[0].deregistration_delay) == "300",
      tostring(aws_lb_target_group.services["legacy"].deregistration_delay) == "300",
      tostring(aws_lb_target_group.services["web"].deregistration_delay) == "300",
    ])
    error_message = "The default deregistration delay must be 300 on the backend and every service target group."
  }

  assert {
    condition     = aws_ecs_service.backend.health_check_grace_period_seconds == null
    error_message = "With the grace period at its default 0, health_check_grace_period_seconds must be null."
  }

  assert {
    condition     = length(aws_ecs_service.backend.load_balancer) == 1
    error_message = "With the ALB on the backend service must carry one load_balancer block."
  }

  assert {
    condition     = aws_ecs_service.backend.id != ""
    error_message = "Turning the ALB on must update aws_ecs_service.backend in place, not plan a new one."
  }

  assert {
    condition     = aws_route53_record.backend[0].id != ""
    error_message = "Turning the ALB on must keep the api_domain record at aws_route53_record.backend[0] and update it in place."
  }

  assert {
    condition     = aws_route53_record.backend[0].alias[0].name == data.aws_lb.alb[0].dns_name
    error_message = "With the ALB on the api_domain record must alias the ALB. Got ${aws_route53_record.backend[0].alias[0].name}."
  }

  assert {
    condition = alltrue([
      length(aws_apigatewayv2_api.api_gateway) == 1,
      length(aws_apigatewayv2_stage.backend) == 1,
      length(aws_cloudwatch_log_group.api_gateway_logs) == 1,
      length(aws_apigatewayv2_vpc_link.backend) == 1,
    ])
    error_message = "Without remove_api_gateway the API, stage, access-log group and backend VPC Link must stay with the ALB on."
  }

  assert {
    condition     = length(aws_apigatewayv2_integration.backend) == 0
    error_message = "With the ALB on the backend must have no API Gateway integration."
  }

  assert {
    condition     = sort(keys(aws_apigatewayv2_route.services)) == tolist(["legacy"])
    error_message = "With the ALB on only the full_open service keeps an API Gateway route. Got ${jsonencode(sort(keys(aws_apigatewayv2_route.services)))}."
  }
}

# ---------------------------------------------------------------------------
# Tuned values reach their attributes.
# ---------------------------------------------------------------------------
run "alb_on_with_tuned_values" {
  command = plan

  variables {
    enable_alb                        = true
    alb_ssl_policy                    = "ELBSecurityPolicy-TLS13-1-2-2021-06"
    alb_deregistration_delay          = 30
    backend_health_check_grace_period = 120
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = aws_lb_listener.https[0].ssl_policy == "ELBSecurityPolicy-TLS13-1-2-2021-06"
    error_message = "alb_ssl_policy must reach the HTTPS listener. Got ${aws_lb_listener.https[0].ssl_policy}."
  }

  assert {
    condition = alltrue([
      tostring(aws_lb_target_group.backend[0].deregistration_delay) == "30",
      tostring(aws_lb_target_group.services["legacy"].deregistration_delay) == "30",
      tostring(aws_lb_target_group.services["web"].deregistration_delay) == "30",
    ])
    error_message = "alb_deregistration_delay = 30 must reach the backend and every service target group."
  }

  assert {
    condition     = aws_ecs_service.backend.health_check_grace_period_seconds == 120
    error_message = "With the ALB on, backend_health_check_grace_period = 120 must reach health_check_grace_period_seconds."
  }
}

# ---------------------------------------------------------------------------
# The grace period applies only with the ALB on.
# ---------------------------------------------------------------------------
run "grace_period_is_ignored_while_the_alb_is_off" {
  command = plan

  variables {
    enable_alb                        = false
    backend_health_check_grace_period = 120
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = aws_ecs_service.backend.health_check_grace_period_seconds == null
    error_message = "With the ALB off the grace period must not be set on the backend service."
  }
}

# ---------------------------------------------------------------------------
# enable_alb = true, remove_api_gateway = true: only the gateway set goes.
# ---------------------------------------------------------------------------
run "alb_on_with_remove_drops_only_the_gateway" {
  command = plan

  variables {
    enable_alb         = true
    remove_api_gateway = true
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = alltrue([
      length(aws_apigatewayv2_api.api_gateway) == 0,
      length(aws_apigatewayv2_stage.backend) == 0,
      length(aws_cloudwatch_log_group.api_gateway_logs) == 0,
      length(aws_apigatewayv2_vpc_link.backend) == 0,
      length(aws_apigatewayv2_integration.backend) == 0,
      length(aws_apigatewayv2_route.backend) == 0,
      length(aws_apigatewayv2_domain_name.backend) == 0,
      length(aws_apigatewayv2_api_mapping.backend) == 0,
    ])
    error_message = "With the ALB on and remove_api_gateway, no API Gateway API, stage, access-log group, backend VPC Link, integration, route, domain name or mapping may be planned."
  }

  # The full_open service would normally keep its route; with no API there is
  # nothing to attach it to.
  assert {
    condition = alltrue([
      length(aws_apigatewayv2_vpc_link.services) == 0,
      length(aws_apigatewayv2_integration.services) == 0,
      length(aws_apigatewayv2_route.services) == 0,
    ])
    error_message = "With the gateway removed no service may keep a VPC Link, integration or route, full_open included. Got routes ${jsonencode(keys(aws_apigatewayv2_route.services))}."
  }

  assert {
    condition     = output.api_gateway_endpoint == "" && output.api_gateway_id == ""
    error_message = "With the gateway removed api_gateway_endpoint and api_gateway_id must be \"\"."
  }

  assert {
    condition     = length(aws_service_discovery_service.backend) == 1 && aws_service_discovery_service.backend[0].id != ""
    error_message = "Removing the gateway must keep the backend's Cloud Map service at its address."
  }

  assert {
    condition     = length(aws_ecs_service.backend.service_registries) == 1
    error_message = "Removing the gateway must keep the backend service's service_registries."
  }

  assert {
    condition     = aws_security_group.backend.id != ""
    error_message = "Removing the gateway must keep the backend security group at its address."
  }

  assert {
    condition     = aws_ecs_service.backend.id != ""
    error_message = "Removing the gateway must update the backend service in place, not plan a new one."
  }

  assert {
    condition     = aws_route53_record.backend[0].alias[0].name == data.aws_lb.alb[0].dns_name
    error_message = "With the gateway removed the api_domain record must alias the ALB. Got ${aws_route53_record.backend[0].alias[0].name}."
  }
}

# ---------------------------------------------------------------------------
# Builds an ALB-mode state for the switch back.
# ---------------------------------------------------------------------------
run "alb_on_applied" {
  command = apply

  variables {
    enable_alb = true
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = length(aws_lb_listener.https) == 1
    error_message = "With the ALB on the HTTPS listener must exist."
  }

  assert {
    condition     = aws_route53_record.backend[0].alias[0].name == data.aws_lb.alb[0].dns_name
    error_message = "With the ALB on the api_domain record must alias the ALB. Got ${aws_route53_record.backend[0].alias[0].name}."
  }
}

# ---------------------------------------------------------------------------
# Turning the ALB off again keeps the record and the backend service.
# ---------------------------------------------------------------------------
run "alb_off_again_updates_the_record_in_place" {
  command = plan

  variables {
    enable_alb = false
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = aws_route53_record.backend[0].id != ""
    error_message = "Turning the ALB off must keep the api_domain record at aws_route53_record.backend[0] and update it in place."
  }

  assert {
    condition     = aws_ecs_service.backend.id != ""
    error_message = "Turning the ALB off must update aws_ecs_service.backend in place, not plan a new one."
  }

  assert {
    condition     = length(aws_ecs_service.backend.load_balancer) == 0
    error_message = "With the ALB off the backend service must have no load_balancer block."
  }

  assert {
    condition     = aws_ecs_service.backend.health_check_grace_period_seconds == null
    error_message = "With the ALB off health_check_grace_period_seconds must be null."
  }
}

# ---------------------------------------------------------------------------
# Validation refusals. Last, because an erroring run skips every later run.
# ---------------------------------------------------------------------------
run "deregistration_delay_out_of_range_is_refused" {
  command = plan

  variables {
    alb_deregistration_delay = 3601
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  expect_failures = [var.alb_deregistration_delay]
}

run "ssl_policy_must_be_a_policy_name" {
  command = plan

  variables {
    alb_ssl_policy = ""
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  expect_failures = [var.alb_ssl_policy]
}

run "negative_grace_period_is_refused" {
  command = plan

  variables {
    backend_health_check_grace_period = -1
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  expect_failures = [var.backend_health_check_grace_period]
}
