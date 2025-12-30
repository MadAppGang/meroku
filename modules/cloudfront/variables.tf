# ============================================================================
# CloudFront Module Variables
# ============================================================================

variable "project" {
  description = "Project name"
  type        = string
}

variable "env" {
  description = "Environment name"
  type        = string
}

variable "name" {
  description = "Unique name for this CloudFront distribution (used to identify multiple distributions)"
  type        = string
  default     = "default"
}

# ============================================================================
# Origin Configuration
# ============================================================================

variable "origins" {
  description = <<-EOT
    List of origins for CloudFront. Each origin should have:
    - name: Unique identifier for the origin
    - type: "s3", "amplify", "alb", or "custom"
    - domain_name: The origin domain name
    - origin_path: (optional) Path to append to origin requests
    - protocol_policy: (optional) "https-only", "http-only", or "match-viewer"
    - custom_headers: (optional) Map of headers to send to origin
    - use_oac: (optional) For S3 origins, whether to use Origin Access Control
  EOT
  type = list(object({
    name              = string
    type              = string  # "s3", "amplify", "alb", "custom"
    domain_name       = string
    origin_path       = optional(string, "")
    http_port         = optional(number, 80)
    https_port        = optional(number, 443)
    protocol_policy   = optional(string, "https-only")
    ssl_protocols     = optional(list(string), ["TLSv1.2"])
    keepalive_timeout = optional(number, 60)
    read_timeout      = optional(number, 60)
    custom_headers    = optional(map(string), {})
    use_oac           = optional(bool, true)
  }))
  default = []
}

# ============================================================================
# Domain Configuration
# ============================================================================

variable "domain_aliases" {
  description = "List of domain aliases (CNAMEs) for CloudFront. Supports wildcards like *.app.domain.com"
  type        = list(string)
  default     = []
}

variable "certificate_arn" {
  description = "ARN of ACM certificate for custom domains (must be in us-east-1)"
  type        = string
  default     = null
}

variable "zone_id" {
  description = "Route53 zone ID for DNS record creation"
  type        = string
  default     = null
}

variable "create_dns_records" {
  description = "Whether to create Route53 records for domain aliases"
  type        = bool
  default     = true
}

# ============================================================================
# Cache Behaviors
# ============================================================================

variable "cache_behaviors" {
  description = <<-EOT
    List of ordered cache behaviors for path-based routing.
    Each behavior should have:
    - path_pattern: Path pattern like "/api/*"
    - origin_name: Name of the origin to route to
    - allowed_methods, cached_methods, forward_query_string, etc.
  EOT
  type = list(object({
    path_pattern           = string
    origin_name            = string
    allowed_methods        = optional(list(string), ["GET", "HEAD"])
    cached_methods         = optional(list(string), ["GET", "HEAD"])
    forward_query_string   = optional(bool, true)
    forward_headers        = optional(list(string), ["Host", "Origin"])
    forward_cookies        = optional(string, "none")
    viewer_protocol_policy = optional(string, "redirect-to-https")
    min_ttl                = optional(number, 0)
    default_ttl            = optional(number, 0)
    max_ttl                = optional(number, 0)
    compress               = optional(bool, true)
  }))
  default = []
}

# ============================================================================
# Default Cache Behavior Settings
# ============================================================================

variable "default_root_object" {
  description = "Default root object for CloudFront"
  type        = string
  default     = "index.html"
}

variable "default_allowed_methods" {
  description = "HTTP methods allowed for default behavior"
  type        = list(string)
  default     = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
}

variable "default_cached_methods" {
  description = "HTTP methods to cache for default behavior"
  type        = list(string)
  default     = ["GET", "HEAD"]
}

variable "forward_query_string" {
  description = "Whether to forward query strings to origin"
  type        = bool
  default     = true
}

variable "forward_headers" {
  description = "Headers to forward to origin (use ['*'] to forward all)"
  type        = list(string)
  default     = ["Host", "Origin", "Access-Control-Request-Headers", "Access-Control-Request-Method"]
}

variable "forward_cookies" {
  description = "Cookie forwarding policy: none, whitelist, or all"
  type        = string
  default     = "none"
}

variable "whitelisted_cookies" {
  description = "List of cookies to whitelist (when forward_cookies = 'whitelist')"
  type        = list(string)
  default     = []
}

variable "viewer_protocol_policy" {
  description = "Protocol policy: allow-all, https-only, or redirect-to-https"
  type        = string
  default     = "redirect-to-https"
}

variable "min_ttl" {
  description = "Minimum TTL for cached objects (seconds)"
  type        = number
  default     = 0
}

variable "default_ttl" {
  description = "Default TTL for cached objects (seconds)"
  type        = number
  default     = 86400  # 1 day
}

variable "max_ttl" {
  description = "Maximum TTL for cached objects (seconds)"
  type        = number
  default     = 31536000  # 1 year
}

variable "compress" {
  description = "Whether to compress content"
  type        = bool
  default     = true
}

# ============================================================================
# SPA Error Handling
# ============================================================================

variable "custom_error_responses" {
  description = <<-EOT
    Custom error responses for SPA routing.
    Default: redirect 403 and 404 to index.html for SPA routing
  EOT
  type = list(object({
    error_code            = number
    response_code         = optional(number)
    response_page_path    = optional(string)
    error_caching_min_ttl = optional(number, 10)
  }))
  default = [
    {
      error_code         = 403
      response_code      = 200
      response_page_path = "/index.html"
    },
    {
      error_code         = 404
      response_code      = 200
      response_page_path = "/index.html"
    }
  ]
}

# ============================================================================
# Edge Functions
# ============================================================================

variable "lambda_function_associations" {
  description = "Lambda@Edge function associations"
  type = list(object({
    event_type   = string  # viewer-request, origin-request, origin-response, viewer-response
    lambda_arn   = string
    include_body = optional(bool, false)
  }))
  default = []
}

variable "cloudfront_function_associations" {
  description = "CloudFront Function associations"
  type = list(object({
    event_type   = string  # viewer-request, viewer-response
    function_arn = string
  }))
  default = []
}

# ============================================================================
# Performance & Security
# ============================================================================

variable "price_class" {
  description = "CloudFront price class: PriceClass_All, PriceClass_200, PriceClass_100"
  type        = string
  default     = "PriceClass_100"  # Use only US and Europe for cost savings
}

variable "geo_restriction_type" {
  description = "Geo restriction type: none, whitelist, or blacklist"
  type        = string
  default     = "none"
}

variable "geo_restriction_locations" {
  description = "List of country codes for geo restriction"
  type        = list(string)
  default     = []
}

variable "web_acl_id" {
  description = "WAF Web ACL ID to associate with CloudFront"
  type        = string
  default     = null
}

# ============================================================================
# Logging
# ============================================================================

variable "logging_bucket" {
  description = "S3 bucket for CloudFront access logs"
  type        = string
  default     = null
}

variable "logging_prefix" {
  description = "Prefix for CloudFront log files"
  type        = string
  default     = null
}

variable "logging_include_cookies" {
  description = "Whether to include cookies in access logs"
  type        = bool
  default     = false
}

# ============================================================================
# Tags
# ============================================================================

variable "tags" {
  description = "Additional tags for resources"
  type        = map(string)
  default     = {}
}
