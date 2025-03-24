resource "aws_iam_role" "task" {
  name               = "${var.project}_${var.task}_task_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role_policy_attachment" "task_cloudwatch" {
  role       = aws_iam_role.task.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role" "task_execution" {
  name               = "${var.project}_scheduler_${var.task}_task_execution_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy_attachment" "task_execution_cloudwatch" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role_policy_attachment" "scheduler" {
  role       = aws_iam_role.scheduler_role.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEventBridgeFullAccess"
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
  name   = "Task${var.task}SSMAccessPolicy"
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

data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "ssm_parameter_access" {
  statement {
    actions   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    resources = ["arn:aws:ssm:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${var.project}/task/${var.task}/*"]
  }
}

resource "aws_iam_role" "scheduler_role" {
  name = "${var.project}_scheduler_${var.task}_role_${var.env}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "scheduler.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "scheduler_ecs_full_access" {
  role       = aws_iam_role.scheduler_role.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonECS_FullAccess"
}


resource "aws_iam_role_policy_attachment" "sqs_access" {
  count      = var.sqs_enable == true ? 1 : 0
  role       = aws_iam_role.task.name
  policy_arn = var.sqs_policy_arn
}
