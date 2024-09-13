variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "schema_file" {
  type    = string
  default = ""
}

variable "vtl_templates_yaml" {
  type    = string
  default = ""
}


variable "auth_lambda_path" {
  type    = string
  default = ""
}


locals {
  vtl_templates  = var.vtl_templates_yaml != "" ? yamldecode(file(var.vtl_templates_yaml)) : yamldecode(file("${path.module}/vtl_templates.yaml"))
  schema_content = var.schema_file != "" ? file(var.schema_file) : file("${path.module}/schema.graphql")
  auth_lambda    = auth_lambda_path != "" ? auth_lambda_path : "${path.module}/auth_lambda"
}
