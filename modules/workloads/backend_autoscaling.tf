# Backend service autoscaling configuration
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
  name               = "${var.project}_backend_cpu_scaling_${var.env}"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.backend[0].resource_id
  scalable_dimension = aws_appautoscaling_target.backend[0].scalable_dimension
  service_namespace  = aws_appautoscaling_target.backend[0].service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value = var.backend_autoscaling_target_cpu
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}

# Memory-based autoscaling policy
resource "aws_appautoscaling_policy" "backend_memory" {
  count              = var.backend_autoscaling_enabled ? 1 : 0
  name               = "${var.project}_backend_memory_scaling_${var.env}"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.backend[0].resource_id
  scalable_dimension = aws_appautoscaling_target.backend[0].scalable_dimension
  service_namespace  = aws_appautoscaling_target.backend[0].service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageMemoryUtilization"
    }
    target_value = var.backend_autoscaling_target_memory
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}

# Optional: Request count based scaling (if ALB is enabled)
resource "aws_appautoscaling_policy" "backend_requests" {
  count              = var.backend_autoscaling_enabled && var.enable_alb ? 1 : 0
  name               = "${var.project}_backend_request_scaling_${var.env}"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.backend[0].resource_id
  scalable_dimension = aws_appautoscaling_target.backend[0].scalable_dimension
  service_namespace  = aws_appautoscaling_target.backend[0].service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ALBRequestCountPerTarget"
      resource_label         = "${aws_lb.backend[0].arn_suffix}/${aws_lb_target_group.backend[0].arn_suffix}"
    }
    target_value = 1000
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}