# The service role every runner project in this environment builds as.
#
# It carries what CodeBuild itself needs to run the job and nothing a workflow
# could use to deploy: no ECR, no ECS, no iam:PassRole. Deploys keep using the
# GitHub OIDC role (modules/workloads/github.tf), which the workflow assumes
# with its own token.
#
# The policy is built with jsonencode rather than aws_iam_policy_document so
# the rendered JSON can be read and asserted on in tests/ with no credentials.

locals {
  # name-or-arn[:json-key[:version-stage[:version-id]]] → the secret part. An
  # ARN has six colons of its own, so it is cut at the seventh.
  secrets_manager_ids = distinct(flatten([
    for r in var.runners : [
      for s in r.secrets : (
        startswith(s.secrets_manager, "arn:")
        ? join(":", slice(split(":", s.secrets_manager), 0, 7))
        : split(":", s.secrets_manager)[0]
      ) if s.secrets_manager != ""
    ]
  ]))

  # Secrets Manager appends a random six-character suffix to every secret ARN.
  # A name matches only through that wildcard; an ARN is granted as given and
  # with the wildcard too, so a partial ARN (no suffix) works as well.
  secrets_manager_arns = flatten([
    for id in local.secrets_manager_ids : (
      startswith(id, "arn:")
      ? [id, "${id}-??????"]
      : ["arn:${local.partition}:secretsmanager:${local.region}:${local.account_id}:secret:${id}-??????"]
    )
  ])

  ssm_parameter_arns = distinct(flatten([
    for r in var.runners : [
      for s in r.secrets : (
        startswith(s.ssm, "arn:")
        ? s.ssm
        : "arn:${local.partition}:ssm:${local.region}:${local.account_id}:parameter/${trimprefix(s.ssm, "/")}"
      ) if s.ssm != ""
    ]
  ]))

  log_group_arns = flatten([
    for name in values(local.log_groups) : [
      "arn:${local.partition}:logs:${local.region}:${local.account_id}:log-group:${name}",
      "arn:${local.partition}:logs:${local.region}:${local.account_id}:log-group:${name}:*",
    ]
  ])

  subnet_arns = [
    for id in var.subnet_ids : "arn:${local.partition}:ec2:${local.region}:${local.account_id}:subnet/${id}"
  ]

  policy_statements = concat(
    [
      {
        Sid      = "WriteRunnerLogs"
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = local.log_group_arns
      },
      {
        Sid      = "ReadSecrets"
        Effect   = "Allow"
        Action   = ["secretsmanager:GetSecretValue"]
        Resource = concat([data.aws_secretsmanager_secret.token.arn], local.secrets_manager_arns)
      },
    ],
    # Optional statements are filtered rather than chosen with `c ? [] : [...]`:
    # tuples of different lengths are different types to a conditional.
    [for st in [
      {
        Sid      = "ReadParameters"
        Effect   = "Allow"
        Action   = ["ssm:GetParameters"]
        Resource = local.ssm_parameter_arns
      },
    ] : st if length(local.ssm_parameter_arns) > 0],
    # What CodeBuild needs to place a build in a VPC:
    # https://docs.aws.amazon.com/codebuild/latest/userguide/auth-and-access-control-iam-identity-based-access-control.html#customer-managed-policies-example-create-vpc-network-interface
    [for st in [
      {
        Sid    = "ManageBuildNetworkInterfaces"
        Effect = "Allow"
        Action = [
          "ec2:CreateNetworkInterface",
          "ec2:DescribeDhcpOptions",
          "ec2:DescribeNetworkInterfaces",
          "ec2:DeleteNetworkInterface",
          "ec2:DescribeSubnets",
          "ec2:DescribeSecurityGroups",
          "ec2:DescribeVpcs",
        ]
        Resource = ["*"]
      },
      {
        Sid      = "AttachBuildNetworkInterfaces"
        Effect   = "Allow"
        Action   = ["ec2:CreateNetworkInterfacePermission"]
        Resource = ["arn:${local.partition}:ec2:${local.region}:${local.account_id}:network-interface/*"]
        Condition = {
          StringEquals = {
            "ec2:Subnet"            = local.subnet_arns
            "ec2:AuthorizedService" = "codebuild.amazonaws.com"
          }
        }
      },
    ] : st if var.vpc_access],
  )

  policy_json = jsonencode({
    Version   = "2012-10-17"
    Statement = local.policy_statements
  })
}

resource "aws_iam_role" "runner" {
  name = module.naming.names["service_role"]

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "codebuild.amazonaws.com" }
      Action    = "sts:AssumeRole"
      # Only CodeBuild acting for this account: the confused-deputy guard.
      Condition = {
        StringEquals = { "aws:SourceAccount" = local.account_id }
      }
    }]
  })
}

resource "aws_iam_role_policy" "runner" {
  name   = "${var.project}_gh_runners_${var.env}"
  role   = aws_iam_role.runner.id
  policy = local.policy_json
}
