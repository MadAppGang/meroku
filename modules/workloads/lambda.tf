
data "archive_file" "lambda" {
  type        = "zip"
  source_file = var.lambda_path
  output_path = "ci_lambda.zip"
}


data "aws_iam_policy_document" "lambda_deploy_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "lambda_deploy_iam" {
  name               = "lambda_deploy_iam_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.lambda_deploy_assume_role.json
}


resource "aws_iam_role_policy_attachment" "lambda_basic_esecution" {
  role       = aws_iam_role.lambda_deploy_iam.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}


resource "aws_lambda_function" "lambda_deploy" {
  filename         = "ci_lambda.zip"
  function_name    = "ci_lambda_${var.env}"
  handler          = "main"
  role             = aws_iam_role.lambda_deploy_iam.arn
  source_code_hash = data.archive_file.lambda.output_base64sha256
  runtime          = "provided.al2"

  environment {
    variables = {
      PROJECT_NAME      = var.project
      SLACK_WEBHOOK_URL = var.slack_deployment_webhook
      PROJECT_ENV       = var.env
    }
  }
}



data "aws_iam_policy_document" "lambda_ecs" {
  statement {
    effect = "Allow"
    actions = [
      "ecs:DescribeTaskDefinition",
      "ecs:ListTaskDefinitions",
      "ecs:UpdateService",
      "iam:PassRole"
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "lambda_ecs" {
  name   = "LambdaECSDevPolicy_${var.env}"
  policy = data.aws_iam_policy_document.lambda_ecs.json
}

resource "aws_iam_role_policy_attachment" "lambda_ecs" {
  role       = aws_iam_role.lambda_deploy_iam.name
  policy_arn = aws_iam_policy.lambda_ecs.arn
}

# Eventbus For ECR
resource "aws_cloudwatch_event_rule" "ecr_event" {
  name        = "ecr_events_cicd"
  description = "Emmit ECR event on new image push"
  event_pattern = jsonencode({
    source = [
      "aws.ecr",
      "aws.ecs",
      "aws.ssm",
      "action.production"
    ]
    detail-type = [
      "ECR Image Action",
      "ECS Deployment State Change",
      "ECS Service Action",
      "Parameter Store Change",
      "DEPLOY"
    ]
  })
}

resource "aws_cloudwatch_event_target" "lambda" {
  rule      = aws_cloudwatch_event_rule.ecr_event.name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}


resource "aws_lambda_permission" "ecr_event_call_deploy_lambda" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.ecr_event.arn
}

