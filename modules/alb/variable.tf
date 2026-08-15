variable "project" {
  type        = string
  description = "Project name"
}

variable "env" {
  type        = string
  description = "Environment name"
}


variable "vpc_id" {
  type        = string
  description = "VPC ID where to create EFS"
}

variable "private_subnets" {
  type        = list(string)
  description = "List of private subnet IDs"
}

# Long-lived connections (SSE, streaming, websockets) are dropped once the ALB sees no
# bytes for idle_timeout seconds. Raise this above the app's heartbeat interval to hold
# them open. API Gateway cannot do this at all — its 30s integration cap is fixed.
# Whether to create the load balancer at all.
#
# The module is instantiated in BOTH ingress modes and switches itself off here,
# rather than being wrapped in a conditional by the caller. That is deliberate and
# is the fix for a destroy-ordering bug:
#
# modules/workloads takes the ALB's security group as a plain variable, so the only
# thing telling Terraform that the backend security group depends on the ALB one is
# that reference. When the caller dropped the whole module on disable, the reference
# went with it — and Terraform, no longer seeing an edge, destroyed the ALB security
# group while the backend rule still pointed at it. AWS refuses that with
# DependencyViolation, and the apply retried for ~14 minutes and then failed.
#
# Keeping the module instantiated keeps the edge, so the backend rule is revoked
# first and the security group deletes cleanly.
variable "enabled" {
  description = "Create the ALB and its security group. False leaves the module in the graph with nothing in it, which preserves dependency ordering on teardown."
  type        = bool
  default     = true
}

variable "idle_timeout" {
  type        = number
  description = "Seconds the ALB holds an idle connection open (AWS default 60; raise for SSE/streaming)"
  default     = 60
}


