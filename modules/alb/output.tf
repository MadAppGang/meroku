# Every output is guarded for var.enabled = false, where the resources have
# count = 0.
#
# The guards are written as explicit conditionals on the resource rather than with
# try(), because the reference is what tells Terraform that a consumer of this
# output depends on these resources. That edge is the entire reason the module
# stays instantiated when disabled: without it, Terraform destroyed the ALB
# security group while the backend still had a rule pointing at it. Losing the
# reference here would reintroduce exactly the bug var.enabled exists to fix.
#
# They return "" rather than null so callers can keep testing != "".

output "alb_arn" {
  value       = length(aws_lb.alb) > 0 ? aws_lb.alb[0].arn : ""
  description = "The ARN of the application load balancer, or \"\" when disabled"
}

output "alb_zone_id" {
  value       = length(aws_lb.alb) > 0 ? aws_lb.alb[0].zone_id : ""
  description = "The Route53 hosted zone ID of the load balancer, for alias records"
}

# Was returning zone_id, like the output above it. Nothing consumed it, so the
# mistake never surfaced.
output "alb_dns_name" {
  value       = length(aws_lb.alb) > 0 ? aws_lb.alb[0].dns_name : ""
  description = "The DNS name of the application load balancer"
}

# The backend's security group needs this to allow ingress from the ALB. The ALB's
# security group lives in this module, not in workloads, so it must be passed across.
#
# Always present, unlike the outputs above: the group is created in both modes so that
# disabling the ALB never destroys something the backend's rule still references. See
# the note on the resource in main.tf.
output "alb_security_group_id" {
  value       = aws_security_group.alb.id
  description = "Security group ID of the application load balancer. Always set — the group exists in both modes."
}
