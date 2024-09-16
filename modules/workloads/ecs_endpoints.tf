data "aws_vpc" "default" {
  default = true
}

data "aws_route_table" "default" {
  vpc_id = var.vpc_id
  filter {
    name   = "association.main"
    values = ["true"]
  }
}

# S3 VPC Endpoint
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = var.vpc_id
  service_name      = "com.amazonaws.${data.aws_region.current.name}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [data.aws_route_table.default.id]

  tags = {
    Name      = "${var.project}-s3-endpoint"
    terraform = "true"
    env       = var.env
  }
}

# DynamoDB VPC Endpoint
resource "aws_vpc_endpoint" "dynamodb" {
  vpc_id            = var.vpc_id
  service_name      = "com.amazonaws.${data.aws_region.current.name}.dynamodb"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [data.aws_route_table.default.id]

  tags = {
    Name      = "${var.project}-dynamodb-endpoint"
    terraform = "true"
    env       = var.env
  }
}

# ECR VPC Endpoints
resource "aws_vpc_endpoint" "ecr_dkr" {
  vpc_id             = var.vpc_id
  service_name       = "com.amazonaws.${data.aws_region.current.name}.ecr.dkr"
  vpc_endpoint_type  = "Interface"
  subnet_ids         = var.subnet_ids
  security_group_ids = [aws_security_group.ecr_endpoint.id]

  private_dns_enabled = true

  tags = {
    Name      = "${var.project}-ecr-dkr-endpoint"
    terraform = "true"
    env       = var.env
  }
}

resource "aws_vpc_endpoint" "ecr_api" {
  vpc_id             = var.vpc_id
  service_name       = "com.amazonaws.${data.aws_region.current.name}.ecr.api"
  vpc_endpoint_type  = "Interface"
  subnet_ids         = var.subnet_ids
  security_group_ids = [aws_security_group.ecr_endpoint.id]

  private_dns_enabled = true

  tags = {
    Name      = "${var.project}-ecr-api-endpoint"
    terraform = "true"
    env       = var.env
  }
}



# Security group for ECR endpoints
resource "aws_security_group" "ecr_endpoint" {
  name        = "${var.project}-ecr-endpoint-sg"
  description = "Allow inbound HTTPS traffic for ECR endpoints"
  vpc_id      = var.vpc_id

  ingress {
    description      = "HTTPS from VPC"
    from_port        = 443
    to_port          = 443
    protocol         = "tcp"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name      = "${var.project}-ecr-endpoint-sg"
    terraform = "true"
    env       = var.env
  }
}
