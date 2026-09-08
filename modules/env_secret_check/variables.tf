variable "workloads" {
  description = <<-EOT
    The container definitions to check, keyed however the caller keys them.

    `environment` and `secrets` must be the names EXACTLY as they are rendered
    into the container definition — `environment[].name` and `secrets[].name`
    — and nothing else. The caller is expected to hand over the same locals the
    resource itself reads, not a re-derivation of them: ECS compares those two
    lists byte for byte, and a check that rebuilds either one is a check that
    can drift away from the thing it is checking.

    That matters most on the secrets side, which is not written down anywhere.
    modules/workloads discovers it, turning every SSM parameter under a path
    into `upper(reverse(split("/", name))[0])` — the UPPER-CASED last segment.
    So a parameter called ".../scan_cursor_store" arrives here as
    "SCAN_CURSOR_STORE", which is what makes it collide with an environment
    variable of that name, and passing the raw parameter path instead would
    miss every collision there is.

    `subject` opens the sentence ("Service \"api\"", "The backend").
    `ssm_path` is the prefix whose parameters become the secrets, named in the
    message so the reader knows where to go looking.
    `yaml_field` and `yaml_file` name the other half — the place the declared
    environment variables come from ("env_vars" in "project/dev.yaml").
  EOT

  type = map(object({
    subject     = string
    ssm_path    = string
    yaml_field  = string
    yaml_file   = string
    environment = list(string)
    secrets     = list(string)
  }))

  default = {}
}
