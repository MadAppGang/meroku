# Test helper for backend_policies.tftest.hcl.
#
# Calls the workloads module the way env/main.hbs does when a custom/pre
# module declares a backend_policies output: workload.policy concatenated with
# that output, referenced directly. The output's resources are unknown at plan
# (terraform_data's id is only assigned at apply), like the ARN of a KMS key a
# custom/pre module creates in the same apply.
#
# Synthetic values only; this repository is public.

module "custom_pre" {
  source = "./pre"
}

module "workloads" {
  source = "../.."

  project          = "acme"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:acme/acme:ref:refs/heads/main"]

  # The workload.policy half in the shape hclArray renders it, with an effect
  # and a condition the custom/pre half does not have.
  backend_policy = concat(
    [
      {
        effect    = "Deny"
        actions   = ["s3:DeleteObject"]
        resources = ["arn:aws:s3:::example-bucket/*"]
        conditions = [
          { test = "StringNotEquals", variable = "aws:RequestedRegion", values = ["us-east-1"] },
        ]
      },
    ],
    module.custom_pre.backend_policies,
  )
}
