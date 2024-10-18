resource "aws_ecs_cluster" "main" {
  name = "${var.project}_cluster_${var.env}"

  tags = {
    terraform = "true"
    env       = var.env
  }
}

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "ecs_tasks_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}



