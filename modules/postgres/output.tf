output "endpoint" {
  value =  aws_db_instance.database.address
}

output "port" {
  value =  aws_db_instance.database.port
}

output "user" {
  value =  var.username
}

output "password" {
  value =  aws_ssm_parameter.postgres_password.value
}

output "db_name" {
  value =  var.db_name
}
