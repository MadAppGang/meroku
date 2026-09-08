# Whether a container definition would name the same variable twice — once in
# `environment` and once in `secrets` — and the sentence shown when it does.
#
# ECS rejects that task definition outright:
#
#   ClientException: The secret name must be unique and not shared with any new
#   or existing environment variables set on the container, such as
#   'SCAN_CURSOR_STORE'.
#
# It is worth being precise about how a project gets there, because the shape of
# the failure is what decides where the check has to live. Two independent
# mechanisms write into one container's environment:
#
#   * DECLARED. project/<env>.yaml — a service's env_vars, or the backend's
#     backend_env_variables — plus the variables meroku always sets itself
#     (AWS_REGION, URL, PG_DATABASE_HOST, EVENT_SOURCE, SERVICE_NAME, and the
#     rest of local.services_env / local.backend_env).
#   * DISCOVERED. Every SSM parameter under /<env>/<project>/<service>, turned
#     into a secret by env_services.tf and env.tf as
#     `upper(reverse(split("/", name))[0])` — the UPPER-CASED last segment.
#
# Neither knows about the other, and only one of them is in a file anybody
# reviews. So the person who breaks the deploy is whoever runs `aws ssm
# put-parameter`, the person who sees it break is whoever runs the next
# `terraform apply`, and the name in the error is one that neither of them
# edited. Nothing about the YAML changed; nothing about the parameter looked
# dangerous.
#
# And it breaks the apply, not the plan: RegisterTaskDefinition is refused
# part-way through, so whatever else that apply was doing is already half done.
# One service's stray parameter takes the whole environment's apply with it.
#
# The names compared are the RENDERED ones — the exact lists that reach
# `environment` and `secrets` in the container definition — which is why
# env_services.tf and env.tf hoist them into locals rather than assembling them
# inline at the resource. See ../env_secret_check for why the comparison and the
# message live in a module with no provider: this one reads eight remote data
# sources and cannot be planned in CI, and `terraform validate` never evaluates
# a precondition's error_message, so a message that could not render would pass
# every gate here and fail in a user's terminal. That is not hypothetical; it
# happened to the compute_pool message in v4.2.0 and shipped through four
# releases.
#
# The two instances differ only in the words and in where the reader is sent.

module "service_env_secret_check" {
  source = "../env_secret_check"

  workloads = { for k, v in local.service_names : k => {
    subject         = "Service \"${k}\""
    ssm_path        = "/${var.env}/${var.project}/${k}"
    yaml_field      = "env_vars"
    yaml_file       = "project/${var.env}.yaml"
    defaults_source = "modules/workloads/env_services.tf"

    environment = [for e in local.services_container_env[k] : e.name]
    secrets     = [for s in local.services_env_ssm[k] : s.name]
  } }
}

module "backend_env_secret_check" {
  source = "../env_secret_check"

  workloads = {
    backend = {
      subject         = "The backend"
      ssm_path        = "/${var.env}/${var.project}/backend"
      yaml_field      = "backend_env_variables"
      yaml_file       = "project/${var.env}.yaml"
      defaults_source = "modules/workloads/env.tf"

      # `try` and `compact`, unlike the services branch, because var.backend_env
      # carries no type constraint (variables.tf) and env/main.hbs concatenates
      # `try(module.custom_pre.backend_env_vars, [])` into it — a list produced
      # by a user-supplied module, whose elements this module has never been in
      # a position to insist on. An element with no `name` is already broken and
      # ECS will say so; it must not turn this check into a plan-time crash that
      # points at the check rather than at the element.
      environment = compact([for e in local.backend_container_env : try(e.name, "")])
      secrets     = [for s in local.backend_env_ssm : s.name]
    }
  }
}
