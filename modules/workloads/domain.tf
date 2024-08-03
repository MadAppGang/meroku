locals {
  domain_name = "${var.env == "prod" ? "api." : format("%s.", var.env)}${var.domain}"
}


resource "aws_route53_record" "backend" {
  name    = "${local.domain_name}"
  type    = "A"
  zone_id = var.domain_zone_id
  alias {
    name                   = aws_apigatewayv2_domain_name.backend.domain_name_configuration[0].target_domain_name
    zone_id                = aws_apigatewayv2_domain_name.backend.domain_name_configuration[0].hosted_zone_id
    evaluate_target_health = true
  }
}

resource "aws_apigatewayv2_domain_name" "backend" {
  domain_name = "${local.domain_name}"
  domain_name_configuration {
    certificate_arn = var.root_certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

resource "aws_apigatewayv2_stage" "backend" {
  api_id      = aws_apigatewayv2_api.api_gateway.id
  name        = var.env
  auto_deploy = true
}

resource "aws_apigatewayv2_api_mapping" "backend" {
  api_id      = aws_apigatewayv2_api.api_gateway.id
  domain_name = aws_apigatewayv2_domain_name.backend.domain_name
  stage       = aws_apigatewayv2_stage.backend.id
}
