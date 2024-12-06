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