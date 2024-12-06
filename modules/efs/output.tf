
# modules/efs/outputs.tf
output "efs_configs" {
  description = "Map of created EFS configurations"
  value = {
    for name, efs in aws_efs_file_system.this : name => {      
      id              = efs.id
      access_point_id = aws_efs_access_point.this[name].id 
      dns_name        = efs.dns_name
      security_group  = aws_security_group.efs[name].id
      path            = aws_efs_access_point.this[name].root_directory.0.path 
    }
  }
}