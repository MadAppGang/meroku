# Backend service autoscaling configuration.
#
# target_value reads var.backend_autoscaling_target_cpu / _target_memory. Both
# variables have always existed (variables.tf) and env/main.hbs has always
# passed them, but nothing read them: the policies below hardcoded 70.0 and
# 75.0, so `backend_autoscaling_target_cpu: 50` in YAML silently scaled at 70.
# The memory policy's literal (75.0) also disagreed with the variable's
# documented default (80); the variable wins, because the variable's default is
# the contract the YAML and the UI describe.
resource "aws_appautoscaling_target" "backend" {
  count              = var.backend_autoscaling_enabled ? 1 : 0
  max_capacity       = var.backend_autoscaling_max_capacity
  min_capacity       = var.backend_autoscaling_min_capacity
  resource_id        = "service/${aws_ecs_cluster.main.name}/${aws_ecs_service.backend.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

# CPU-based autoscaling policy
resource "aws_appautoscaling_policy" "backend_cpu" {
  count              = var.backend_autoscaling_enabled ? 1 : 0
  name               = "cpu-scaling-policy"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.backend[0].resource_id
  scalable_dimension = aws_appautoscaling_target.backend[0].scalable_dimension
  service_namespace  = aws_appautoscaling_target.backend[0].service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = var.backend_autoscaling_target_cpu
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}

# Memory-based autoscaling policy
resource "aws_appautoscaling_policy" "backend_memory" {
  count              = var.backend_autoscaling_enabled ? 1 : 0
  name               = "memory-scaling-policy"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.backend[0].resource_id
  scalable_dimension = aws_appautoscaling_target.backend[0].scalable_dimension
  service_namespace  = aws_appautoscaling_target.backend[0].service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageMemoryUtilization"
    }
    target_value       = var.backend_autoscaling_target_memory
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
    disable_scale_in   = false
  }
}

# Applies backend_desired_count to the running service while autoscaling is
# off. aws_ecs_service.backend ignores desired_count after create (see its
# lifecycle block), so this is what carries a changed count to the service.
# With autoscaling on there is no instance and Application Auto Scaling owns
# the count.
#
# It runs when it is created (a new environment, autoscaling turned off, the
# first apply after this resource was added) and when it is replaced (the
# count, cluster or service changed). It is keyed on the configured count, not
# the live one, so with autoscaling off a count changed outside Terraform is
# not reverted until the configured count changes.
#
# When autoscaling is turned off, the scalable target is deregistered in the
# same apply with no ordering against this step, so a scale action in those
# few seconds could overwrite the configured count.
#
# Only the task count changes, so no new deployment starts, and the CLI returns
# as soon as ECS accepts the request. Needs /bin/sh and the AWS CLI. Credentials
# are inherited from the terraform process, as in modules/ses/main.tf: no
# --profile is passed, so the CLI resolves the credentials the AWS provider
# does. Names go in through the environment, so the shell never parses them.
resource "terraform_data" "backend_desired_count" {
  count = var.backend_autoscaling_enabled ? 0 : 1

  triggers_replace = {
    desired_count = var.backend_desired_count
    cluster       = aws_ecs_cluster.main.name
    service       = aws_ecs_service.backend.name
  }

  provisioner "local-exec" {
    interpreter = ["env", "-u", "NEON_API_KEY", "-u", "TF_VAR_neon_pg_credentials", "/bin/sh", "-c"]
    command     = "aws ecs update-service --cluster \"$ECS_CLUSTER\" --service \"$ECS_SERVICE\" --desired-count \"$DESIRED_COUNT\" --query service.desiredCount --output text"
    environment = {
      ECS_CLUSTER   = aws_ecs_cluster.main.name
      ECS_SERVICE   = aws_ecs_service.backend.name
      DESIRED_COUNT = tostring(var.backend_desired_count)
      AWS_REGION    = data.aws_region.current.region
      AWS_PAGER     = ""
    }
  }
}
