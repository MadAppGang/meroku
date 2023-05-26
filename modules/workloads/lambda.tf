data "archive_file" "ci_lambda" {
  type        = "zip"
  source_dir  = "./ci_lambda"
  output_path = "./ci_lambda.zip"
}

data "aws_s3_bucket" "lambda_bucket" {
  bucket = "${var.project}-ci_lambda-${var.env}${var.image_bucket_postfix}"
  acl    = "private"
  tags = {
    terraform = "true"
    env       = var.env
  }
}

resource "aws_s3_bucket_object" "object" {
  bucket = aws_s3_bucket.lambda_bucket.id
  key    = "ci_lambda.zip"
  source = data.archive_file.ci_lambda.output_path
  acl    = "private"
  tags = {
    terraform = "true"
    env       = var.env
  }
}

resource "aws_lambda_function" "ci_lambda" {
  function_name = "${var.project}-ci_lambda"

  s3_bucket     = aws_s3_bucket.lambda_bucket.id
  s3_key        = aws_s3_bucket_object.object.key

  handler = "index.handler"
  runtime = "nodejs18.x"

  role = "role_arn"

  source_code_hash = data.archive_file.ci_lambda.output_base64sha256
}