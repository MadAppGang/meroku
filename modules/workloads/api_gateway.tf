locals {
  # remove_api_gateway takes effect only with the ALB on; with the ALB off it is ignored,
  # because the gateway is then the only ingress. Every gateway resource's count ANDs
  # this into its own condition, so each [0] reference is safe from that resource's
  # count alone.
  api_gateway_enabled = !(var.enable_alb && var.remove_api_gateway)
}

# The API and the backend VPC Link gained count when remove_api_gateway was added; the
# moved blocks keep each environment's existing, unindexed object instead of replacing
# it. Keep them for as long as any environment may still hold the unindexed address.
moved {
  from = aws_apigatewayv2_api.api_gateway
  to   = aws_apigatewayv2_api.api_gateway[0]
}

resource "aws_apigatewayv2_api" "api_gateway" {
  count = local.api_gateway_enabled ? 1 : 0

  name          = "${var.project}-${var.env}"
  protocol_type = "HTTP"

  tags = {
    Name        = "${var.project}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# VPC Links for services. Only for services whose network_access puts them on
# API Gateway; see local.service_on_api_gateway in services.tf. Every pre-v30
# service is full_open, which is always on it, so the set is unchanged for them.
resource "aws_apigatewayv2_vpc_link" "services" {
  for_each = { for k, v in local.service_names : k => v if local.service_on_api_gateway[k] }

  name               = "${var.project}-${each.key}-${var.env}"
  security_group_ids = [aws_security_group.services[each.key].id]
  subnet_ids         = var.subnet_ids

  tags = {
    Name        = "${var.project}-${each.key}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

moved {
  from = aws_apigatewayv2_vpc_link.backend
  to   = aws_apigatewayv2_vpc_link.backend[0]
}

resource "aws_apigatewayv2_vpc_link" "backend" {
  count = local.api_gateway_enabled ? 1 : 0

  name               = "${var.project}-${var.env}"
  security_group_ids = [aws_security_group.backend.id]
  subnet_ids         = var.subnet_ids

  tags = {
    Name        = "${var.project}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


# Integrations for services
resource "aws_apigatewayv2_integration" "services" {
  for_each = { for k, v in local.service_names : k => v if local.service_on_api_gateway[k] }

  api_id             = aws_apigatewayv2_api.api_gateway[0].id
  integration_type   = "HTTP_PROXY"
  integration_method = "ANY"
  integration_uri    = aws_service_discovery_service.services[each.key].arn
  connection_id      = aws_apigatewayv2_vpc_link.services[each.key].id
  connection_type    = "VPC_LINK"

  # Forward the path to the backend service
  request_parameters = {
    "overwrite:path" = "/$request.path.proxy"
  }
}

# Routes for services
resource "aws_apigatewayv2_route" "services" {
  for_each = { for k, v in local.service_names : k => v if local.service_on_api_gateway[k] }

  api_id    = aws_apigatewayv2_api.api_gateway[0].id
  route_key = "ANY /${each.key}/{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.services[each.key].id}"
}

# The backend integration routes through Cloud Map. Cloud Map itself is now created in BOTH
# ingress modes (backend.tf keeps count = 1 unconditionally, because AWS will not let
# service_registries be removed from a running service). Only the API Gateway wiring is
# conditional, which is what these counts express: with the ALB on, nothing here is created,
# so nothing indexes aws_service_discovery_service.backend[0] — the "Invalid index" this
# gating originally fixed.
resource "aws_apigatewayv2_integration" "backend" {
  count = !var.enable_alb && local.api_gateway_enabled ? 1 : 0

  api_id             = aws_apigatewayv2_api.api_gateway[0].id
  integration_type   = "HTTP_PROXY"
  integration_method = "ANY"
  integration_uri    = aws_service_discovery_service.backend[0].arn
  connection_id      = aws_apigatewayv2_vpc_link.backend[0].id
  connection_type    = "VPC_LINK"

  # Forward the path to the backend service
  request_parameters = {
    "overwrite:path" = "/$request.path.proxy"
  }
}

resource "aws_apigatewayv2_route" "backend" {
  count = !var.enable_alb && local.api_gateway_enabled ? 1 : 0

  api_id    = aws_apigatewayv2_api.api_gateway[0].id
  route_key = "ANY /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.backend[0].id}"
}
