variable "project" {
  description = "Project name"
  type        = string
}

variable "env" {
  description = "Environment name (dev, staging, prod)"
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for VPC (creates 2 AZs with public subnets, plus private subnets when a NAT egress strategy is selected)"
  type        = string
  default     = "10.0.0.0/16"
}

variable "egress_strategy" {
  description = <<-EOT
    How ECS tasks reach the internet.

      public_ip      Every task gets its own public IPv4 address ($3.65/mo each).
                     No private subnets, no NAT. Cheapest below roughly 5
                     services, and the default.

      nat_gateway    Tasks run in private subnets behind a single NAT Gateway
                     (~$32.85/mo flat + $0.045/GB). Cheaper than public IPs past
                     roughly 5 services, and tasks lose their public addresses.
                     The NAT is zonal, so losing its AZ takes out all egress.

      nat_gateway_ha One NAT Gateway per AZ (~$65.70/mo flat + $0.045/GB).
                     Survives the loss of an AZ and avoids cross-AZ transfer
                     charges. Recommended for production past roughly 10
                     services.

    Switching is a one-line change and can be done at any time; it replaces the
    tasks' network configuration, so expect a rolling redeploy. See
    ai_docs/EGRESS_COST_MODEL.md for the full cost model.
  EOT
  type        = string
  default     = "public_ip"

  validation {
    condition     = contains(["public_ip", "nat_gateway", "nat_gateway_ha"], var.egress_strategy)
    error_message = "egress_strategy must be one of: public_ip, nat_gateway, nat_gateway_ha."
  }
}
