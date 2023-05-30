variable "enable_mobile_client" {
  default = true
  type    = bool
}

variable "enable_web_client" {
  default = true
  type    = bool
}

variable "user_pool_domain_prefix" {
  default = ""
}

variable "enable_user_pool_domain" {
  type    = bool
  default = false
}

variable "web_callback_urls" {
  type    = list(string)
  default = ["https://jwt.io"]
}

variable "web_callback" {
  type    = string
  default = "https://jwt.io"
}


variable "mobile_callback_urls" {
  type    = list(string)
  default = ["https://jwt.io"]
}

variable "mobile_callback" {
  type    = string
  default = "https://jwt.io"
}

variable "enable_dashboard_client" {
  default = false
  type    = bool
}

variable "dashboard_callback_urls" {
  type    = list(string)
  default = ["https://jwt.io"]
}

variable "auto_verified_attributes" {
  type    = list(string)
  default = ["email"]
}

variable "allow_backend_task_to_confirm_signup" {
  default = false
}

variable "backend_task_role_name" {
  type    = string
}

variable "env" {
  type    = string
  default = "dev"
}

variable "project" {
  type = string
}