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
variable "idle_timeout" {
  type        = number
  description = "Seconds the ALB holds an idle connection open (AWS default 60; raise for SSE/streaming)"
  default     = 60
}


