
variable "domain_zone" {
  type    = string
}
  
variable "env" {
  type    = string
}

#v2

variable "create_domain_zone" {
  type = bool
}

variable "add_env_domain_prefix" {
  type = bool
  default = true
}

variable "api_domain_prefix" {
  type = string
  default = "api"
}
