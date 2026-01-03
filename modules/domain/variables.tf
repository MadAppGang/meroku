
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

variable "project" {
  type = string
}

variable "force_destroy" {
  description = "Allow deletion of Route53 zone even when it contains records (e.g., Amplify ACM validation records). Required when changing domain names."
  type        = bool
  default     = true
}
