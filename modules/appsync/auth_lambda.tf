locals {
  # Build output lives OUTSIDE the authorizer source tree. Zipping the source
  # directory in place used to sweep yarn.lock, .yarnrc.yml, README.md,
  # index.test.mjs and the previous zip into the deployed artifact.
  auth_lambda_build_dir   = "${path.module}/.build"
  auth_lambda_stage_dir   = "${path.module}/.build/auth_lambda"
  auth_lambda_zip         = "${path.module}/.build/auth_lambda.zip"
  auth_lambda_stage_probe = "${path.module}/.build/auth_lambda/index.mjs"

  # Hash every file in the authorizer source directory, not just index.mjs and
  # package.json, so any change to what gets packaged forces a rebuild.
  # fileset(dir, "*") lists top-level files only, so node_modules is excluded;
  # its contents are pinned by yarn.lock, which is hashed here.
  auth_lambda_source_hashes = {
    for file in fileset(local.auth_lambda, "*") :
    file => filemd5("${local.auth_lambda}/${file}")
  }

  # Stage only what the Lambda runtime needs: the handler, its manifest and the
  # installed dependencies.
  auth_lambda_build_command = <<-EOT
    set -eu
    src="${local.auth_lambda}"
    stage="${local.auth_lambda_stage_dir}"
    build="${local.auth_lambda_build_dir}"

    rm -rf "$stage"
    mkdir -p "$stage"

    # Keep the build directory out of git without touching the repo .gitignore.
    printf '*\n' > "$build/.gitignore"

    # path.module is relative, so resolve the destination before changing
    # directories or the copies below land in the wrong place.
    stage="$(cd "$stage" && pwd)"

    cd "$src"
    if [ -f yarn.lock ]; then
      yarn install --immutable
    else
      yarn install
    fi

    # archive_file normalises timestamps, so rebuilding an unchanged source tree
    # yields a byte-identical zip and no spurious Lambda redeployment.
    cp -p index.mjs package.json "$stage/"
    cp -Rp node_modules "$stage/node_modules"
  EOT
}

# Everything below exists only for auth_mode = "lambda". In cognito and oidc mode
# AWS verifies the token itself, so there is no function to build, package, pay
# for or cold-start — and no yarn install during terraform apply.

# Install dependencies and stage the deployment package
resource "null_resource" "auth_lambda_build" {
  count = local.use_lambda_auth ? 1 : 0

  triggers = merge(
    local.auth_lambda_source_hashes,
    {
      build_command = md5(local.auth_lambda_build_command)

      # A fresh clone (CI) has no build directory even though the source hashes
      # are unchanged. Without this the archive step would fail or package a
      # stale artifact.
      staged = fileexists(local.auth_lambda_stage_probe) ? "present" : "absent"
    }
  )

  provisioner "local-exec" {
    command = local.auth_lambda_build_command
  }
}

# Archive the staged Lambda function code
data "archive_file" "lambda_zip" {
  count = local.use_lambda_auth ? 1 : 0

  type        = "zip"
  source_dir  = local.auth_lambda_stage_dir
  output_path = local.auth_lambda_zip

  # This ensures the archive is created after the package has been staged
  depends_on = [null_resource.auth_lambda_build]
}

# Create an IAM role for the Lambda function
resource "aws_iam_role" "lambda_role" {
  count = local.use_lambda_auth ? 1 : 0

  name = module.naming.names["lambda_exec_role"]

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
  count = local.use_lambda_auth ? 1 : 0

  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
  role       = aws_iam_role.lambda_role[0].name
}

# Create the Lambda function
resource "aws_lambda_function" "function" {
  count = local.use_lambda_auth ? 1 : 0

  filename         = data.archive_file.lambda_zip[0].output_path
  function_name    = module.naming.names["auth_lambda"]
  description      = "AppSync Lambda authorizer: verifies RS256 JWTs against ${var.jwks_uri}"
  role             = aws_iam_role.lambda_role[0].arn
  handler          = "index.handler"
  source_code_hash = data.archive_file.lambda_zip[0].output_base64sha256
  runtime          = "nodejs20.x"

  # AppSync gives a Lambda authorizer 10 seconds. The authorizer gives up on the
  # JWKS endpoint after 3s, so it always answers (with a deny) before this.
  timeout = 10

  environment {
    variables = merge(
      {
        # Required. The authorizer denies every request when this is missing;
        # it has no built-in fallback issuer by design.
        JWKS_URI = var.jwks_uri
      },
      var.jwt_issuer == "" ? {} : { JWT_ISSUER = var.jwt_issuer },
      var.jwt_audience == "" ? {} : { JWT_AUDIENCE = var.jwt_audience },
      # The claim policy. Omitted entirely when empty so the function's
      # environment shows at a glance whether one is in force. The handler denies
      # every request if this is present but unparseable, rather than treating a
      # broken policy as no policy.
      length(var.required_claims) == 0 ? {} : { REQUIRED_CLAIMS = jsonencode(var.required_claims) },
    )
  }

  tags = {
    Name        = "${var.project}-${var.env}-appsync-auth"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# AppSync cannot invoke the authorizer without an explicit invoke permission
resource "aws_lambda_permission" "appsync_invoke" {
  count = local.use_lambda_auth ? 1 : 0

  statement_id  = "AllowAppSyncInvokeAuthorizer"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.function[0].function_name
  principal     = "appsync.amazonaws.com"
  source_arn    = aws_appsync_graphql_api.pubsub.arn
}
