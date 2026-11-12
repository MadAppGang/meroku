# API Gateway and the ALB are mutually exclusive ingress paths, and both serve var.api_domain.
# One record serves the name in both modes. Only its alias target differs, so flipping
# enable_alb is a single in-place UPSERT of this record: the name never stops resolving,
# which it did when each mode had its own record and the switch was a delete then a create.
locals {
  # Each mode's condition exactly as it was when the two modes had separate records.
  api_domain_record_enabled = var.api_domain != "" && (
    var.enable_alb ? true : (var.enable_custom_domain && var.create_api_domain_record)
  )
}

# On the API Gateway branch the record's count implies the domain name's count
# (enable_custom_domain && !enable_alb && api_domain != ""), so [0] exists there; on the
# ALB branch data.aws_lb.alb has count = enable_alb. A plain conditional, not try(), so the
# reference gives Terraform the dependency edge.
resource "aws_route53_record" "backend" {
  count   = local.api_domain_record_enabled ? 1 : 0
  name    = var.api_domain
  type    = "A"
  zone_id = var.domain_zone_id
  alias {
    name                   = var.enable_alb ? data.aws_lb.alb[0].dns_name : aws_apigatewayv2_domain_name.backend[0].domain_name_configuration[0].target_domain_name
    zone_id                = var.enable_alb ? data.aws_lb.alb[0].zone_id : aws_apigatewayv2_domain_name.backend[0].domain_name_configuration[0].hosted_zone_id
    evaluate_target_health = true
  }
}

# ALB-mode environments held this record as aws_route53_record.alb_alias in alb.tf. The two
# old resources had mutually exclusive counts, so the target address is never occupied when
# the old one holds state. Keep this for as long as any environment may still hold that address.
moved {
  from = aws_route53_record.alb_alias
  to   = aws_route53_record.backend
}

resource "aws_apigatewayv2_domain_name" "backend" {
  count       = local.api_gateway_enabled && var.enable_custom_domain && !var.enable_alb && var.api_domain != "" ? 1 : 0
  domain_name = var.api_domain
  domain_name_configuration {
    certificate_arn = var.api_certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }

  tags = {
    Name        = "${var.project}-${var.env}-api-domain"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


# The access-log group and the stage exist only with the API. They gained count when
# remove_api_gateway was added; the moved blocks keep each environment's existing,
# unindexed object instead of replacing it. Keep them for as long as any environment
# may still hold the unindexed address.
moved {
  from = aws_cloudwatch_log_group.api_gateway_logs
  to   = aws_cloudwatch_log_group.api_gateway_logs[0]
}

resource "aws_cloudwatch_log_group" "api_gateway_logs" {
  count = local.api_gateway_enabled ? 1 : 0

  name              = "/aws/api_gateway/${aws_apigatewayv2_api.api_gateway[0].name}"
  retention_in_days = 30

  tags = {
    Name        = "/aws/api_gateway/${aws_apigatewayv2_api.api_gateway[0].name}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

moved {
  from = aws_apigatewayv2_stage.backend
  to   = aws_apigatewayv2_stage.backend[0]
}

resource "aws_apigatewayv2_stage" "backend" {
  count = local.api_gateway_enabled ? 1 : 0

  api_id      = aws_apigatewayv2_api.api_gateway[0].id
  name        = var.env
  auto_deploy = true
  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.api_gateway_logs[0].arn
    format = jsonencode({
      requestId      = "$context.requestId",
      sourceIp       = "$context.identity.sourceIp",
      requestTime    = "$context.requestTime",
      protocol       = "$context.protocol",
      httpMethod     = "$context.httpMethod",
      resourcePath   = "$context.resourcePath",
      routeKey       = "$context.routeKey",
      status         = "$context.status",
      responseLength = "$context.responseLength",
    })
  }
  default_route_settings {
    throttling_burst_limit = 5000
    throttling_rate_limit  = 10000
  }

  tags = {
    Name        = var.env
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_apigatewayv2_api_mapping" "backend" {
  count       = local.api_gateway_enabled && var.enable_custom_domain && !var.enable_alb && var.api_domain != "" ? 1 : 0
  api_id      = aws_apigatewayv2_api.api_gateway[0].id
  domain_name = aws_apigatewayv2_domain_name.backend[0].domain_name
  stage       = aws_apigatewayv2_stage.backend[0].id
}
