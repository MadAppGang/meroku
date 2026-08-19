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