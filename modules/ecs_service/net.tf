resource "aws_security_group" "service" {
  name   = "${var.project}_service_${var.service}_${var.env}" 
  vpc_id = var.vpc_id

  tags = {
    Name        = "${var.project}-${var.service}-service-sg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  egress {
    protocol         = "-1"
    from_port        = 0
    to_port          = 0
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }
}


