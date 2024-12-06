data "aws_caller_identity" "current" {}

resource "aws_iam_role" "task" {
  name               = "${var.project}_${var.service}_service_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role_policy_attachment" "task_cloudwatch" {
  role       = aws_iam_role.task.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role" "task_execution" {
  name               = "${var.project}_service_${var.service}_task_execution_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "ecs_tasks_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

// SSM IAM access policy
resource "aws_iam_policy" "ssm_parameter_access" {
  name   = "Service${var.service}SSMAccessPolicy"
  policy = data.aws_iam_policy_document.ssm_parameter_access.json
}

resource "aws_iam_role_policy_attachment" "ssm_parameter_access_task" {
  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
  depends_on = [aws_iam_policy.ssm_parameter_access]
}

resource "aws_iam_role_policy_attachment" "ssm_parameter_access_task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
  depends_on = [aws_iam_policy.ssm_parameter_access]
}


data "aws_iam_policy_document" "ssm_parameter_access" {
  statement {
    actions   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    resources = ["arn:aws:ssm:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${var.project}/service/${var.service}/*"]
  }
}


// SQS IAM access policy
resource "aws_iam_role_policy_attachment" "sqs_access" {
  count      = var.sqs_enable == true ? 1 : 0
  role       = aws_iam_role.task.name
  policy_arn = var.sqs_policy_arn
}
