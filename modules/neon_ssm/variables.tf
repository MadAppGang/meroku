variable "project" {
  description = "meroku project name; the second segment of each parameter path."
  type        = string
}

variable "env" {
  description = "meroku environment name; the first segment of each parameter path."
  type        = string
}

variable "services" {
  description = "Services that get /<env>/<project>/<svc>/database_url."
  type        = list(string)
}

variable "urls" {
  description = "Service name => DATABASE_URL, merged from every neon_access module."
  type        = map(string)
  sensitive   = true
}
