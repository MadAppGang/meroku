resource "aws_apigatewayv2_api" "api_gateway" {
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

# VPC Links for services
resource "aws_apigatewayv2_vpc_link" "services" {
  for_each = local.service_names

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

resource "aws_apigatewayv2_vpc_link" "backend" {
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
  for_each = local.service_names

  api_id             = aws_apigatewayv2_api.api_gateway.id
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
  for_each = local.service_names

  api_id    = aws_apigatewayv2_api.api_gateway.id
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
  count = var.enable_alb ? 0 : 1

  api_id             = aws_apigatewayv2_api.api_gateway.id
  integration_type   = "HTTP_PROXY"
  integration_method = "ANY"
  integration_uri    = aws_service_discovery_service.backend[0].arn
  connection_id      = aws_apigatewayv2_vpc_link.backend.id
  connection_type    = "VPC_LINK"

  # Forward the path to the backend service
  request_parameters = {
    "overwrite:path" = "/$request.path.proxy"
  }
}

resource "aws_apigatewayv2_route" "backend" {
  count = var.enable_alb ? 0 : 1

  api_id    = aws_apigatewayv2_api.api_gateway.id
  route_key = "ANY /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.backend[0].id}"
}
