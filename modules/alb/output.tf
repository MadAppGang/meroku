output "alb_arn" {
  value       = aws_lb.alb.arn
  description = "The ARN of the application load balancer"
}

output "alb_zone_id" {
  value       = aws_lb.alb.zone_id
  description = "The Zone ID of the application load balancer"
}

output "alb_dns_name" {
  value       = aws_lb.alb.zone_id
  description = "The Zone ID of the application load balancer"
}

# The backend's security group needs this to allow ingress from the ALB. The ALB's
# security group lives in this module, not in workloads, so it must be passed across.
output "alb_security_group_id" {
  value       = aws_security_group.alb.id
  description = "Security group ID of the application load balancer"
}