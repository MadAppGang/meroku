output "endpoint" {
  value = var.aurora ? aws_rds_cluster.aurora[0].endpoint : aws_db_instance.database[0].address
}

output "reader_endpoint" {
  value = var.aurora ? aws_rds_cluster.aurora[0].reader_endpoint : null
}

output "port" {
  value = var.aurora ? aws_rds_cluster.aurora[0].port : aws_db_instance.database[0].port
}

output "user" {
  value = var.username
}

output "password" {
  value = aws_ssm_parameter.postgres_password.value
}

output "db_name" {
  value = var.db_name
}

output "is_aurora" {
  value = var.aurora
}