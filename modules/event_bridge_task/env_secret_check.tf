# Whether this event task's container definition would name the same variable
# twice — once in `environment` and once in `secrets` — and the sentence shown
# when it does.
#
# ECS rejects that task definition outright:
#
#   ClientException: The secret name must be unique and not shared with any new
#   or existing environment variables set on the container, such as
#   'SCAN_CURSOR_STORE'.
#
# The shape here is the one modules/workloads has, with the same two independent
# mechanisms writing into one container's environment and the same nobody in
# charge of the pair:
#
#   * DECLARED. environment_variables on this task's entry in
#     project/<env>.yaml, rendered by env/main.hbs into var.custom_env_vars,
#     concatenated after the variables this module always sets itself —
#     AWS_REGION, SQS_QUEUE_URL, EVENT_BUS_NAME, EVENT_SOURCE, API_DOMAIN,
#     PRIVATE_DNS_NAMESPACE, BACKEND_INTERNAL_URL (env.tf, local.default_env_vars).
#   * DISCOVERED. Every SSM parameter under /<env>/<project>/task/<task>, turned
#     into a secret by env.tf as `upper(reverse(split("/", name))[0])` — the
#     UPPER-CASED last segment. Byte for byte the derivation modules/workloads
#     uses, which is what lets one check module serve both.
#
# So the person who breaks the deploy is whoever runs `aws ssm put-parameter`,
# the person who sees it break is whoever runs the next `terraform apply`, and
# the name in the error is one that neither of them edited.
#
# Two things about the paths differ from a service's and both reach the reader:
# the SSM prefix carries an extra "task" segment, and the YAML field is
# `environment_variables` rather than `env_vars`. `defaults_source` is this
# module's own env.tf, because the always-set list above is the one remedy the
# message can offer for a name the user never wrote — and sending that reader to
# modules/workloads/env_services.tf, which sets none of these for their task,
# would leave them with nowhere to go at all.
#
# The names compared are the RENDERED ones — the exact lists that reach
# `environment` and `secrets` in the container definition. No `try` or `compact`
# on the environment side, unlike the backend's: var.custom_env_vars is typed
# `list(object({ name = string, value = string }))` (variables.tf), so every
# element has a name and an element that does not is rejected before this runs.
#
# See ../env_secret_check for why the comparison and the message live in a
# module with no provider.
module "task_env_secret_check" {
  source = "../env_secret_check"

  workloads = {
    task = {
      subject         = "Event task \"${var.task}\""
      ssm_path        = "/${var.env}/${var.project}/task/${var.task}"
      yaml_field      = "environment_variables"
      yaml_file       = "project/${var.env}.yaml"
      defaults_source = "modules/event_bridge_task/env.tf"

      environment = [for e in local.environment_variables : e.name]
      secrets     = [for s in local.task_env_ssm : s.name]
    }
  }
}
