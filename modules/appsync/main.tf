resource "aws_appsync_graphql_api" "pubsub" {
  name                = "${var.project}-${var.env}-pubsub-api"
  authentication_type = "AWS_LAMBDA"
  schema              = file(var.vtl_templates_yaml)

  lambda_authorizer_config {
    authorizer_uri = var.lambda_function_arn
  }

  additional_authentication_provider {
    authentication_type = "API_KEY"
  }
}

resource "aws_appsync_api_key" "pubsub" {
  api_id  = aws_appsync_graphql_api.pubsub.id
  expires = timeadd(timestamp(), "8760h")  # 1 year from now
}

resource "aws_iam_role" "appsync" {
  name = "${var.project}-${var.env}-appsync-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "appsync.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "appsync_logs" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSAppSyncPushToCloudWatchLogs"
  role       = aws_iam_role.appsync.name
}



resource "aws_appsync_datasource" "none" {
  api_id = aws_appsync_graphql_api.pubsub.id
  name   = "None"
  type   = "NONE"
}


# Create the AppSync resolvers
resource "aws_appsync_resolver" "resolvers" {
  for_each = merge([
    for type, fields in local.vtl_templates : {
      for field, templates in fields : 
      "${type}.${field}" => {
        type = type
        field = field
        request = templates.request
        response = templates.response
      }
    }
  ]...)

  api_id      = aws_appsync_graphql_api.pubsub.id
  type        = each.value.type
  field       = each.value.field
  data_source = aws_appsync_datasource.none.name

  request_template  = each.value.request
  response_template = each.value.response
}

