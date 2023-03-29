resource "aws_cognito_user_pool" "user_pool" {
  auto_verified_attributes   = ["email"]
  deletion_protection        = "INACTIVE"
  email_verification_message = "Your verification code is {####}"
  email_verification_subject = "Your verification code"

  mfa_configuration          = "OPTIONAL"
  name                       = "${var.project}-user-pool-${var.env}"
  sms_authentication_message = "Your login code is {####}"
  sms_verification_message   = "Your verification code is {####}"
  tags                       = {}

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
    attributes_require_verification_before_update = ["email"]
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
