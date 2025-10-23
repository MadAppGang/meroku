package main

// getSampleTerraformPlan returns a realistic sample terraform plan JSON for testing
func getSampleTerraformPlan() string {
	return `{
  "format_version": "1.2",
  "terraform_version": "1.9.0",
  "variables": {
    "region": {
      "value": "us-east-1"
    },
    "environment": {
      "value": "dev"
    }
  },
  "planned_values": {
    "root_module": {
      "resources": [
        {
          "address": "aws_vpc.main",
          "mode": "managed",
          "type": "aws_vpc",
          "name": "main",
          "provider_name": "registry.terraform.io/hashicorp/aws",
          "values": {
            "cidr_block": "10.0.0.0/16",
            "enable_dns_hostnames": true,
            "enable_dns_support": true
          }
        },
        {
          "address": "aws_subnet.public[0]",
          "mode": "managed",
          "type": "aws_subnet",
          "name": "public",
          "provider_name": "registry.terraform.io/hashicorp/aws",
          "values": {
            "cidr_block": "10.0.1.0/24",
            "availability_zone": "us-east-1a"
          }
        },
        {
          "address": "aws_ecs_cluster.main",
          "mode": "managed",
          "type": "aws_ecs_cluster",
          "name": "main",
          "provider_name": "registry.terraform.io/hashicorp/aws",
          "values": {
            "name": "dev-cluster"
          }
        },
        {
          "address": "aws_ecs_service.backend",
          "mode": "managed",
          "type": "aws_ecs_service",
          "name": "backend",
          "provider_name": "registry.terraform.io/hashicorp/aws",
          "values": {
            "name": "backend-service",
            "desired_count": 2
          }
        },
        {
          "address": "aws_db_instance.postgres",
          "mode": "managed",
          "type": "aws_db_instance",
          "name": "postgres",
          "provider_name": "registry.terraform.io/hashicorp/aws",
          "values": {
            "engine": "postgres",
            "instance_class": "db.t3.micro"
          }
        }
      ]
    }
  },
  "resource_changes": [
    {
      "address": "aws_vpc.main",
      "mode": "managed",
      "type": "aws_vpc",
      "name": "main",
      "provider_config_key": "aws",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "cidr_block": "10.0.0.0/16",
          "enable_dns_hostnames": true,
          "enable_dns_support": true,
          "tags": {
            "Name": "dev-vpc",
            "Environment": "dev",
            "ManagedBy": "terraform"
          }
        }
      }
    },
    {
      "address": "aws_subnet.public[0]",
      "mode": "managed",
      "type": "aws_subnet",
      "name": "public",
      "provider_config_key": "aws",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "cidr_block": "10.0.1.0/24",
          "availability_zone": "us-east-1a",
          "map_public_ip_on_launch": true,
          "tags": {
            "Name": "dev-public-subnet-1"
          }
        }
      }
    },
    {
      "address": "aws_subnet.public[1]",
      "mode": "managed",
      "type": "aws_subnet",
      "name": "public",
      "provider_config_key": "aws",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "cidr_block": "10.0.2.0/24",
          "availability_zone": "us-east-1b",
          "map_public_ip_on_launch": true,
          "tags": {
            "Name": "dev-public-subnet-2"
          }
        }
      }
    },
    {
      "address": "aws_internet_gateway.main",
      "mode": "managed",
      "type": "aws_internet_gateway",
      "name": "main",
      "provider_config_key": "aws",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "tags": {
            "Name": "dev-igw"
          }
        }
      }
    },
    {
      "address": "aws_ecs_cluster.main",
      "mode": "managed",
      "type": "aws_ecs_cluster",
      "name": "main",
      "provider_config_key": "aws",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "name": "dev-cluster",
          "tags": {
            "Environment": "dev"
          }
        }
      }
    },
    {
      "address": "aws_ecs_service.backend",
      "mode": "managed",
      "type": "aws_ecs_service",
      "name": "backend",
      "provider_config_key": "aws",
      "change": {
        "actions": ["update"],
        "before": {
          "desired_count": 1,
          "deployment_minimum_healthy_percent": 50
        },
        "after": {
          "desired_count": 2,
          "deployment_minimum_healthy_percent": 100,
          "enable_execute_command": true
        }
      }
    },
    {
      "address": "aws_ecs_task_definition.backend",
      "mode": "managed",
      "type": "aws_ecs_task_definition",
      "name": "backend",
      "provider_config_key": "aws",
      "change": {
        "actions": ["delete", "create"],
        "before": {
          "family": "backend",
          "cpu": "256",
          "memory": "512"
        },
        "after": {
          "family": "backend",
          "cpu": "512",
          "memory": "1024",
          "requires_compatibilities": ["FARGATE"]
        }
      }
    },
    {
      "address": "aws_db_instance.postgres",
      "mode": "managed",
      "type": "aws_db_instance",
      "name": "postgres",
      "provider_config_key": "aws",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "allocated_storage": 20,
          "engine": "postgres",
          "engine_version": "15.4",
          "instance_class": "db.t3.micro",
          "identifier": "dev-postgres",
          "username": "admin",
          "multi_az": false,
          "publicly_accessible": false,
          "storage_encrypted": true
        }
      }
    },
    {
      "address": "aws_security_group.alb",
      "mode": "managed",
      "type": "aws_security_group",
      "name": "alb",
      "provider_config_key": "aws",
      "change": {
        "actions": ["delete"],
        "before": {
          "name": "old-alb-sg",
          "description": "Old ALB security group"
        },
        "after": null
      }
    },
    {
      "address": "aws_cloudwatch_log_group.backend",
      "mode": "managed",
      "type": "aws_cloudwatch_log_group",
      "name": "backend",
      "provider_config_key": "aws",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "name": "/ecs/backend",
          "retention_in_days": 30
        }
      }
    },
    {
      "address": "aws_route53_record.api",
      "mode": "managed",
      "type": "aws_route53_record",
      "name": "api",
      "provider_config_key": "aws",
      "change": {
        "actions": ["update"],
        "before": {
          "name": "api.example.com",
          "type": "A",
          "ttl": 300
        },
        "after": {
          "name": "api.example.com",
          "type": "A",
          "ttl": 60
        }
      }
    }
  ],
  "output_changes": {
    "vpc_id": {
      "actions": ["create"]
    },
    "cluster_name": {
      "actions": ["create"]
    }
  }
}`
}
