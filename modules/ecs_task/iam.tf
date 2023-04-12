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

resource "aws_iam_role_policy_attachment" "scheduler" {
  role       = aws_iam_role.task_execution.name
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


  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type = "Service"
      identifiers = ["scheduler.amazonaws.com"]
    }
  }
}

