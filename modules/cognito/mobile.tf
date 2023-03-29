resource "aws_cognito_user_pool_client" "mobile" {
  name                                 = "mobile"
  allowed_oauth_flows_user_pool_client = true
  count                                = var.enable_mobile_client ? 1 : 0

  user_pool_id                 = aws_cognito_user_pool.user_pool.id
  explicit_auth_flows          = ["ALLOW_CUSTOM_AUTH", "ALLOW_REFRESH_TOKEN_AUTH", "ALLOW_USER_PASSWORD_AUTH", "ALLOW_USER_SRP_AUTH"]
  supported_identity_providers = ["COGNITO"]
  allowed_oauth_flows          = ["implicit", "code"]
  allowed_oauth_scopes         = ["aws.cognito.signin.user.admin", "openid", "email", "phone", "profile"]
  callback_urls                = var.mobile_callback_urls
  default_redirect_uri         = var.mobile_callback

  read_attributes = [
    "address",
    "birthdate",
    "custom:test",
    "email",
    "email_verified",
    "family_name",
    "gender",
    "given_name",
    "locale",
    "middle_name",
    "name",
    "nickname",
    "phone_number",
    "phone_number_verified",
    "picture",
    "preferred_username",
    "profile",
    "updated_at",
    "website",
    "zoneinfo"
  ]

  write_attributes = [
    "address",
    "birthdate",
    "custom:test",
    "email",
    "family_name",
    "gender",
    "given_name",
    "locale",
    "middle_name",
    "name",
    "nickname",
    "phone_number",
    "picture",
    "preferred_username",
    "profile",
    "updated_at",
    "website",
    "zoneinfo"
  ]

}
