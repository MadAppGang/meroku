# Every capped AWS name this module builds, in one place. See modules/naming.
#
# This is the registry. If you are adding a resource whose name AWS caps, add it
# here rather than interpolating a template at the resource — that is how the
# 32-character target-group failure got in, and how it would get back in.
#
# Not listed: names with a 128-character cap (the IAM policies) or none at all
# (security groups, log groups, task definitions, SSM parameters). Those have
# hundreds of characters of headroom and routing them through here would be
# noise. The cutoff is 80.

module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = merge(
    {
      # ---- ALB target groups, 32 characters -----------------------------
      backend_tg = {
        legacy = "${var.project}-backend-tg-${var.env}"
        parts  = ["backend"]
        limit  = 32
      }
      backend_tg_instance = {
        legacy = "${var.project}-backend-inst-tg-${var.env}"
        parts  = ["backend", "inst"]
        limit  = 32
      }

      # ---- S3, 63 characters --------------------------------------------
      # The postfix is a random suffix that makes the bucket globally unique,
      # so it must survive truncation like a .fifo suffix does.
      backend_bucket = {
        legacy = "${var.project}-backend-${var.env}${var.backend_bucket_postfix}"
        parts  = ["backend"]
        limit  = 63
        suffix = var.backend_bucket_postfix
      }

      # ---- IAM roles, 64 characters -------------------------------------
      backend_task_role = {
        legacy    = "${var.project}_backend_task_${var.env}"
        parts     = ["backend", "task"]
        limit     = 64
        separator = "_"
      }
      backend_task_execution_role = {
        legacy    = "${var.project}_backend_task_execution_${var.env}"
        parts     = ["backend", "task", "execution"]
        limit     = 64
        separator = "_"
      }
      pgadmin_task_role = {
        legacy    = "${var.project}_pgadmin_task_${var.env}"
        parts     = ["pgadmin", "task"]
        limit     = 64
        separator = "_"
      }
      pgadmin_task_execution_role = {
        legacy    = "${var.project}_pgadmin_task_execution_${var.env}"
        parts     = ["pgadmin", "task", "execution"]
        limit     = 64
        separator = "_"
      }
      ecs_instance_role = {
        legacy    = "${var.project}_ecs_instance_${var.env}"
        parts     = ["ecs", "instance"]
        limit     = 64
        separator = "_"
      }
      lambda_deploy_role = {
        legacy    = "${var.project}_lambda_deploy_iam_${var.env}"
        parts     = ["lambda", "deploy", "iam"]
        limit     = 64
        separator = "_"
      }
      # env in the middle; legacy wins while it fits.
      github_actions_role = {
        legacy = "${var.project}-${var.env}-github-actions-role"
        parts  = ["github", "actions", "role"]
        limit  = 64
      }

      # ---- Lambda and EventBridge, 64 characters ------------------------
      ci_lambda = {
        legacy    = "${var.project}_ci_lambda_${var.env}"
        parts     = ["ci", "lambda"]
        limit     = 64
        separator = "_"
      }
      ci_ecr_rule = {
        legacy    = "${var.project}_ci_ecr_${var.env}"
        parts     = ["ci", "ecr"]
        limit     = 64
        separator = "_"
      }
      ci_ecs_rule = {
        legacy    = "${var.project}_ci_ecs_${var.env}"
        parts     = ["ci", "ecs"]
        limit     = 64
        separator = "_"
      }
      ci_ssm_rule = {
        legacy    = "${var.project}_ci_ssm_${var.env}"
        parts     = ["ci", "ssm"]
        limit     = 64
        separator = "_"
      }
      ci_manual_rule = {
        legacy    = "${var.project}_ci_manual_${var.env}"
        parts     = ["ci", "manual"]
        limit     = 64
        separator = "_"
      }
      ci_manual_global_rule = {
        legacy    = "${var.project}_ci_manual_global_${var.env}"
        parts     = ["ci", "manual", "global"]
        limit     = 64
        separator = "_"
      }
    },

    # ---- S3 env-file EventBridge rules, 64 characters -------------------
    #
    # These used to carry their own truncate-and-hash, written out inline in
    # lambda.tf — the same idea as modules/naming, implemented a second time.
    # `legacy` reproduces exactly what that code emitted so no deployed rule is
    # renamed, and everything from here on is the shared cascade.
    #
    # The name is global to the account and region, and PutRule is an upsert, so
    # two projects declaring the same bucket and key would silently share one
    # rule. That is why project and env are in the name at all, and why the
    # digest covers them.
    { for k, v in local.s3_event_rule_keys : "s3_env_rule_${k}" => {
      legacy = "s3-env-${var.project}-${var.env}-${substr(v.sanitized, 0, 12)}-${v.hash}"
      parts  = ["s3", "env", v.sanitized]
      limit  = 64
    } },

    # ---- Per-service target groups, 32 characters -----------------------
    #
    # Both variants are declared for every service even though only one is
    # created, because the pair must stay distinct while a service is being
    # flipped between Fargate and EC2 and both groups briefly exist. The
    # module's `collisions` output is asserted in services.tf for exactly this.
    { for k in keys(local.service_names) : "service_tg_${k}" => {
      legacy = "${var.project}-service-${k}-tg-${var.env}"
      parts  = [k]
      limit  = 32
    } },
    { for k in keys(local.service_names) : "service_tg_instance_${k}" => {
      legacy = "${var.project}-svc-${k}-i-${var.env}"
      parts  = [k, "i"]
      limit  = 32
    } },

    # ---- Per-service IAM roles, 64 characters ---------------------------
    { for k in keys(local.service_names) : "service_task_role_${k}" => {
      legacy    = "${var.project}_service_${k}_task_${var.env}"
      parts     = ["service", k, "task"]
      limit     = 64
      separator = "_"
    } },
    { for k in keys(local.service_names) : "service_task_execution_role_${k}" => {
      legacy    = "${var.project}_service_${k}_task_execution_${var.env}"
      parts     = ["service", k, "task", "execution"]
      limit     = 64
      separator = "_"
    } },
  )
}
