output "vpc_id" {
  description = "ID of the VPC"
  value       = aws_vpc.main.id
}

output "vpc_cidr" {
  description = "CIDR block of the VPC"
  value       = aws_vpc.main.cidr_block
}

output "public_subnet_ids" {
  description = "IDs of public subnets"
  value       = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  description = "IDs of private subnets (empty unless a NAT egress strategy is selected)"
  value       = aws_subnet.private[*].id
}

output "subnet_ids" {
  description = "All subnet IDs (public subnets). Kept for callers that place internet-facing resources such as ALBs."
  value       = aws_subnet.public[*].id
}

# The two outputs below are the ones ECS task modules should consume. Reading
# them instead of branching on egress_strategy keeps the decision in one place.

output "task_subnet_ids" {
  description = "Subnets ECS tasks should run in: private when a NAT strategy is selected, public otherwise"
  value       = local.needs_private_subnets ? aws_subnet.private[*].id : aws_subnet.public[*].id
}

output "assign_public_ip" {
  description = "Whether ECS tasks need a public IPv4 address. False whenever a NAT provides egress instead."
  value       = !local.needs_private_subnets
}

output "egress_strategy" {
  description = "The egress strategy actually in effect"
  value       = var.egress_strategy
}

output "nat_gateway_ids" {
  description = "IDs of the NAT Gateways (empty for the public_ip strategy)"
  value       = aws_nat_gateway.main[*].id
}

output "internet_gateway_id" {
  description = "ID of the Internet Gateway"
  value       = aws_internet_gateway.main.id
}

output "s3_gateway_endpoint_id" {
  description = "ID of the free S3 gateway endpoint"
  value       = aws_vpc_endpoint.s3.id
}
