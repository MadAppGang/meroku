

variable "enable_mobile_client" {
  default = true
  type    = bool
}

variable "enable_web_client" {
  default = true
  type    = bool
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

variable "env" {
  type    = string
  default = "dev"
}

variable "domain" {
  type = string
}

variable "project" {
  type = string
}
