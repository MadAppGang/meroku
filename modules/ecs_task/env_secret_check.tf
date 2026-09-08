# Whether this scheduled task's container definition would name the same
# variable twice — once in `environment` and once in `secrets` — and the
# sentence shown when it does.
#
# ECS rejects that task definition outright:
#
#   ClientException: The secret name must be unique and not shared with any new
#   or existing environment variables set on the container, such as
#   'SCAN_CURSOR_STORE'.
#
# The third caller of the same shape, after modules/workloads and
# modules/event_bridge_task, with the same two independent mechanisms writing
# into one container's environment and the same nobody in charge of the pair:
#
#   * DECLARED. environment_variables on this task's entry in the
#     scheduled_tasks list of project/<env>.yaml, rendered by env/main.hbs into
#     var.custom_env_vars, concatenated after the variables this module always
#     sets itself — AWS_REGION, SQS_QUEUE_URL, EVENT_BUS_NAME, EVENT_SOURCE,
#     API_DOMAIN, PRIVATE_DNS_NAMESPACE, BACKEND_INTERNAL_URL (env.tf,
#     local.default_env_vars).
#   * DISCOVERED. Every SSM parameter under /<env>/<project>/task/<task>, turned
#     into a secret by env.tf as `upper(reverse(split("/", name))[0])` — the
#     UPPER-CASED last segment. Byte for byte the derivation the other two use,
#     which is what lets one check module serve all three.
#
# So the person who breaks the deploy is whoever runs `aws ssm put-parameter`,
# the person who sees it break is whoever runs the next `terraform apply`, and
# the name in the error is one that neither of them edited.
#
# local.environment_variables is NEVER empty here, which makes the exposure
# unconditional: the seven defaults above go on every scheduled task whether the
# user asked for them or not, so `aws ssm put-parameter` on
# /<env>/<project>/task/<task>/aws_region breaks the next apply of a task whose
# environment_variables are empty and whose YAML nobody has touched.
#
# `defaults_source` is this module's own env.tf. It reads almost identically to
# modules/event_bridge_task/env.tf — the SSM prefix is in fact the SAME string,
# so a scheduled task and an event processor task sharing a name share their
# discovered secrets too — but the lists are not interchangeable: EVENT_SOURCE
# is "<project>.task.<task>" here and "<project>.event.<task>" there. The reader
# sent to this file is the one with no YAML edit available, so they must land on
# the file that actually sets the name in their error.
#
# The names compared are the RENDERED ones — the exact lists that reach
# `environment` and `secrets` in the container definition. No `try` or `compact`
# on the environment side, unlike the backend's: var.custom_env_vars is typed
# `list(object({ name = string, value = string }))` (variable.tf), so every
# element has a name and an element that does not is rejected before this runs.
#
# See ../env_secret_check for why the comparison and the message live in a
# module with no provider.
module "task_env_secret_check" {
  source = "../env_secret_check"

  workloads = {
    task = {
      subject         = "Scheduled task \"${var.task}\""
      ssm_path        = "/${var.env}/${var.project}/task/${var.task}"
      yaml_field      = "environment_variables"
      yaml_file       = "project/${var.env}.yaml"
      defaults_source = "modules/ecs_task/env.tf"

      environment = [for e in local.environment_variables : e.name]
      secrets     = [for s in local.task_env_ssm : s.name]
    }
  }
}
