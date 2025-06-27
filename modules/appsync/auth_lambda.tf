# Install dependencies
resource "null_resource" "install_dependencies" {
  provisioner "local-exec" {
    command = "cd ${local.auth_lambda} && yarn install"
  }
  triggers = {
    dependencies_versions = filemd5("${local.auth_lambda}/package.json")
    index_file_hash       = filemd5("${local.auth_lambda}/index.mjs")
  }
}

# Archive the Lambda function code
data "archive_file" "lambda_zip" {
  type        = "zip"
  source_dir  = local.auth_lambda
  output_path = "${local.auth_lambda}/../auth_lambda.zip"

  # This ensures the archive is created after dependencies are installed
  depends_on = [null_resource.install_dependencies]
}

# Create an IAM role for the Lambda function
resource "aws_iam_role" "lambda_role" {
  name = "${var.project}-${var.env}-appsync-lambda-exec"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Name        = "${var.project}-${var.env}-appsync-lambda-exec"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Attach basic Lambda execution policy to the IAM role
resource "aws_iam_role_policy_attachment" "lambda_policy" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
  role       = aws_iam_role.lambda_role.name
}

# Create the Lambda function
resource "aws_lambda_function" "function" {
  filename         = data.archive_file.lambda_zip.output_path
  function_name    = "${var.project}-${var.env}-appsync-auth"
  role             = aws_iam_role.lambda_role.arn
  handler          = "index.handler" # Adjust this to match your function's entry point
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
  runtime          = "nodejs20.x" # Adjust this to match your Lambda's runtime

  environment {
    variables = {
      # Add any environment variables your Lambda needs
      EXAMPLE_VAR = "example_value"
    }
  }

  tags = {
    Name        = "${var.project}-${var.env}-appsync-auth"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}
