data "archive_file" "ci_lambda" {
  type        = "zip"
  source_dir  = "./ci_lambda/main"
  output_path = "./ci_lambda.zip"
}

resource "aws_s3_bucket" "lambda_bucket" {
  bucket = "${var.project}-ci_lambda-${var.env}${var.image_bucket_postfix}"
}

resource "aws_s3_bucket_ownership_controls" "lambda_bucket" {
  bucket = aws_s3_bucket.lambda_bucket.id
  rule {
    object_ownership = "ObjectWriter"
  }
}

resource "aws_s3_bucket_acl" "lambda_bucket" {
  bucket = aws_s3_bucket.lambda_bucket.id
  acl    = "private"
  depends_on = [aws_s3_bucket_ownership_controls.lambda_bucket]
}


resource "aws_s3_bucket_object" "object" {
  bucket = aws_s3_bucket.lambda_bucket.id
  key    = "ci_lambda.zip"
  source = data.archive_file.ci_lambda.output_path
  acl    = "private"
}

resource "aws_lambda_function" "ci_lambda" {
  function_name = "${var.project}-ci_lambda"

  s3_bucket     = aws_s3_bucket.lambda_bucket.id
  s3_key        = aws_s3_bucket_object.object.key

  handler = "index.handler"
  runtime = "nodejs18.x"
  environment {
    variables = {
      PROJECT_NAME = var.project
      SLACK_WEBHOOK_URL = var.slack_deployment_webhook
    }
  }
  role = "role_arn"

  source_code_hash = data.archive_file.ci_lambda.output_base64sha256
}