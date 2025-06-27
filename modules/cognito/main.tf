data "aws_region" "current" {}

data "aws_caller_identity" "current" {}

resource "aws_cognito_user_pool" "user_pool" {
  auto_verified_attributes   = var.auto_verified_attributes
  deletion_protection        = "INACTIVE"
  email_verification_message = "Your verification code is {####}"
  email_verification_subject = "Your verification code"

  mfa_configuration          = "OPTIONAL"
  name                       = "${var.project}-user-pool-${var.env}"
  sms_authentication_message = "Your login code is {####}"
  sms_verification_message   = "Your verification code is {####}"
  tags = {
    Name        = "${var.project}-user-pool-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  admin_create_user_config {
    allow_admin_create_user_only = false
  }

  email_configuration {
    email_sending_account = "COGNITO_DEFAULT"
  }

  password_policy {
    minimum_length                   = 8
    require_lowercase                = false
    require_numbers                  = false
    require_symbols                  = false
    require_uppercase                = false
    temporary_password_validity_days = 7
  }

  schema {
    attribute_data_type      = "String"
    developer_only_attribute = false
    mutable                  = true
    name                     = "email"
    required                 = true

    string_attribute_constraints {
      max_length = "2048"
      min_length = "0"
    }
  }
  schema {
    attribute_data_type      = "String"
    developer_only_attribute = false
    mutable                  = true
    name                     = "test"
    required                 = false
    string_attribute_constraints {}
  }

  # sms_configuration {
  #     external_id    = "blockt32804a41_role_external_id"
  #     sns_caller_arn = "arn:aws:iam::927350483665:role/sns32804a4185856-prod"
  #     sns_region     = "us-east-1"
  # }

  software_token_mfa_configuration {
    enabled = true
  }

  user_attribute_update_settings {
    attributes_require_verification_before_update = var.auto_verified_attributes
  }

  username_configuration {
    case_sensitive = false
  }

  # verification_message_template {
  #     default_email_option = "CONFIRM_WITH_CODE"
  #     email_message        = "Your verification code is {####}"
  #     email_subject        = "Your verification code"
  #     sms_message          = "Your verification code is {####}"
  # }

  # output:
  # endpoint                   = "cognito-idp.us-east-1.amazonaws.com/us-east-1_yq0Fy8jdP"
  # id                         = "us-east-1_yq0Fy8jdP"
  # domain                     = "blocktreedev75cd8691-75cd8691-prod"
}


resource "aws_cognito_user_pool_domain" "cognito_domain" {
  count        = (var.enable_user_pool_domain) ? 1 : 0
  domain       = "${var.user_pool_domain_prefix}-${var.project}-${var.env}"
  user_pool_id = aws_cognito_user_pool.user_pool.id
}

resource "aws_cognito_user_group" "main_group" {
  name         = "test"
  user_pool_id = aws_cognito_user_pool.user_pool.id
  description  = "Test user group"
}

resource "aws_cognito_user_group" "admin_group" {
  name         = "admin"
  user_pool_id = aws_cognito_user_pool.user_pool.id
  description  = "Admin user group"
}

resource "aws_iam_policy" "allow_admin_confirm_signup_policy" {
  name   = "AllowAdminConfirmSignUpForBackend"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "cognito-idp:AdminConfirmSignUp"
      ],
      "Resource": [
        "arn:aws:cognito-idp:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:userpool/${aws_cognito_user_pool.user_pool.id}"
      ]
    }
  ]
}
EOF
  tags = {
    Name        = "AllowAdminConfirmSignUpForBackend-${var.project}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "allow_backend_task_to_confirm_cognito_user_signup" {
  count      = var.allow_backend_task_to_confirm_signup ? 1 : 0
  role       = "${var.backend_task_role_name}"
  policy_arn = aws_iam_policy.allow_admin_confirm_signup_policy.arn
}