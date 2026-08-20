# VPC Module - Creates a custom VPC with 2 public subnets (simplified architecture)
# Hardcoded to 2 AZs for simplicity - covers 99% of use cases

locals {
  az_count = 2 # Hardcoded for simplicity - 2 AZs is minimum for HA

  # Egress strategy decides whether tasks get their own public IPv4 address or
  # sit in private subnets behind a NAT Gateway. See ai_docs/EGRESS_COST_MODEL.md
  # for the cost model and the switch thresholds.
  #
  # public_ip      - no private subnets, no NAT. Cheapest below ~5 services.
  # nat_gateway    - private subnets, ONE NAT. Cheapest past the threshold, but
  #                  the NAT is zonal, so one AZ failure takes out all egress.
  # nat_gateway_ha - private subnets, one NAT per AZ. Survives an AZ loss and
  #                  avoids cross-AZ transfer, at twice the hourly rate.
  needs_private_subnets = var.egress_strategy != "public_ip"
  nat_gateway_count     = var.egress_strategy == "nat_gateway_ha" ? local.az_count : (var.egress_strategy == "nat_gateway" ? 1 : 0)
}

data "aws_availability_zones" "available" {
  state = "available"
}

# Main VPC
resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name        = "${var.project}-vpc-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Internet Gateway
resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name        = "${var.project}-igw-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Public Subnets (2 AZs)
resource "aws_subnet" "public" {
  count                   = local.az_count
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, count.index)
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true

  tags = {
    Name        = "${var.project}-public-subnet-${count.index + 1}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
    Type        = "public"
  }
}

# Public Route Table
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = {
    Name        = "${var.project}-public-rt-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Associate public subnets with public route table
resource "aws_route_table_association" "public" {
  count          = local.az_count
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# ---------------------------------------------------------------------------
# Private egress path
#
# Everything below exists only when egress_strategy is a NAT mode. Subnets and
# route tables are free either way, but they are still created conditionally so
# a public_ip environment's plan stays empty rather than showing infrastructure
# nothing uses. Only the NAT Gateway and its Elastic IP cost money.
# ---------------------------------------------------------------------------

data "aws_region" "current" {}

# Private subnets, one per AZ. Offset by 10 in the CIDR so the public subnets
# can grow later without colliding.
resource "aws_subnet" "private" {
  count             = local.needs_private_subnets ? local.az_count : 0
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + 10)
  availability_zone = data.aws_availability_zones.available.names[count.index]

  # No public addressing here by definition - that is the point of the tier.
  map_public_ip_on_launch = false

  tags = {
    Name        = "${var.project}-private-subnet-${count.index + 1}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
    Type        = "private"
  }
}

# Elastic IPs for the NAT Gateways. Each carries the same $0.005/hr public IPv4
# charge a task address would, which is part of why one NAT only pays off once
# it replaces several task addresses.
resource "aws_eip" "nat" {
  count  = local.nat_gateway_count
  domain = "vpc"

  tags = {
    Name        = "${var.project}-nat-eip-${count.index + 1}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# NAT Gateways live in the PUBLIC subnets - they need a route to the IGW in
# order to forward traffic outward.
resource "aws_nat_gateway" "main" {
  count         = local.nat_gateway_count
  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id

  tags = {
    Name        = "${var.project}-nat-${count.index + 1}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  # A NAT Gateway cannot forward anything until the IGW exists.
  depends_on = [aws_internet_gateway.main]
}

# One private route table per AZ. In HA mode each points at its own zone's NAT,
# which keeps traffic in-zone and avoids the $0.01/GB-each-way cross-AZ charge.
# In single-NAT mode they all point at the one NAT, and cross-AZ transfer for
# the other zone is the price of the cheaper topology.
resource "aws_route_table" "private" {
  count  = local.needs_private_subnets ? local.az_count : 0
  vpc_id = aws_vpc.main.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main[min(count.index, local.nat_gateway_count - 1)].id
  }

  tags = {
    Name        = "${var.project}-private-rt-${count.index + 1}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_route_table_association" "private" {
  count          = local.needs_private_subnets ? local.az_count : 0
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}

# S3 Gateway Endpoint - free, which is why it is unconditional.
#
# Gateway endpoints, unlike interface endpoints, carry no hourly charge and no
# per-GB charge. In a public_ip environment it changes nothing measurable. In a
# NAT environment it keeps S3 traffic off the NAT entirely - and that includes
# every ECR image layer, the largest single component of task egress - avoiding
# $0.045/GB for no cost at all.
resource "aws_vpc_endpoint" "s3" {
  vpc_id = aws_vpc.main.id
  # .name rather than .region: newer provider versions deprecate it, but the
  # version this repo pins does not expose .region yet, and every other module
  # here uses .name. Change all 37 call sites together, not this one alone.
  service_name      = "com.amazonaws.${data.aws_region.current.name}.s3"
  vpc_endpoint_type = "Gateway"

  route_table_ids = concat(
    [aws_route_table.public.id],
    aws_route_table.private[*].id,
  )

  tags = {
    Name        = "${var.project}-s3-endpoint-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}
