# Test-only external discovery stand-in: output stays unknown during a fresh plan.
terraform {
  required_version = ">= 1.10.0"
}

variable "vault_secret_name" {
  type    = string
  default = "ENV"
}

resource "terraform_data" "discovery" {
  input = [{ name = "UNRELATED", origin = "/dev/myapp/api/unrelated" }]
}

module "check" {
  source = "../../.."

  workloads = {
    fresh = {
      subject         = "Service \"api\""
      ssm_path        = "/dev/myapp/api"
      yaml_field      = "env_vars"
      yaml_file       = "project/dev.yaml"
      defaults_source = "modules/workloads/env_services.tf"
      environment     = ["AWS_REGION"]
      secrets = concat(
        ["ENV", var.vault_secret_name],
        [for s in terraform_data.discovery.output : s.name]
      )
      known_secrets = [
        { name = "ENV", origin = "/dev/myapp/api/env" },
        { name = var.vault_secret_name, origin = "/dev/myapp/vault/payments/env" }
      ]
      discovered_secrets = terraform_data.discovery.output
    }
  }
}

output "valid_known" {
  value = module.check.valid_known
}

output "message_known" {
  value = module.check.message_known
}
