resource "aws_apigatewayv2_api" "api_gateway" {
  name          = "${var.project}-${var.env}"
  protocol_type = "HTTP"
}


resource "aws_apigatewayv2_vpc_link" "backend" {
  name               = "${var.project}-${var.env}"
  security_group_ids = [aws_security_group.backend.id]
  subnet_ids         = var.subnet_ids
}


resource "aws_apigatewayv2_integration" "backend" {
  api_id             = aws_apigatewayv2_api.api_gateway.id
  integration_type   = "HTTP_PROXY"
  integration_method = "ANY"
  integration_uri    = data.aws_service_discovery_service.backend.arn
  connection_id      = aws_apigatewayv2_vpc_link.backend.id
  connection_type    = "VPC_LINK"
}


resource "aws_apigatewayv2_route" "backend" {
  api_id    = aws_apigatewayv2_api.api_gateway.id
  route_key = "ANY /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.backend.id}"
}

