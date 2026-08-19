
# The VPC's own CIDR, so the default ingress rule below can be scoped to it.
# Same pattern and same data source name as modules/workloads/backend.tf.
data "aws_vpc" "selected" {
  id = var.vpc_id
}

resource "aws_security_group" "database" {
  name   = "${var.project}-postgres-${var.env}"
  vpc_id = var.vpc_id

  # 5432 is open to the internet ONLY when the operator explicitly asked for a
  # publicly addressable instance. This rule used to be 0.0.0.0/0 + ::/0
  # unconditionally, which made the database's credentials the only control on
  # every environment generated from the shipped sample.
  #
  # Tightening it takes nothing away from an existing deployment: with
  # public_access = false the instance has no public address (main.tf passes the
  # same variable to publicly_accessible), so the open rule could never have been
  # reached from outside the VPC. Callers inside the VPC — ECS tasks, pgAdmin,
  # anything on a peered path terminating in-VPC — keep working unchanged.
  #
  # With public_access = true the open rule stays, because that is then a named
  # choice the operator made rather than a default nobody chose.
  ingress {
    protocol         = "tcp"
    from_port        = 5432
    to_port          = 5432
    cidr_blocks      = var.public_access ? ["0.0.0.0/0"] : [data.aws_vpc.selected.cidr_block]
    ipv6_cidr_blocks = var.public_access ? ["::/0"] : []
  }

  egress {
    protocol         = "-1"
    from_port        = 0
    to_port          = 0
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = {
    Name        = "${var.project}-postgres-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}
