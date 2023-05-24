
variable domain {
  type = string
}

variable env {
  type = string
}

variable test_emails {
  type = list(string)
  default = [ ]
}

variable "zone_id" {
  type = string
}