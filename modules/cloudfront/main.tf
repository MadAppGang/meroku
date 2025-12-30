# ============================================================================
# CloudFront Distribution Module
# Supports multiple origin types: S3, Amplify, ALB, Custom URL
# ============================================================================

locals {
  # Generate unique origin IDs for each origin (include distribution name for uniqueness)
  origin_configs = {
    for origin in var.origins : origin.name => merge(origin, {
      origin_id = "${var.project}-${var.env}-${var.name}-${origin.name}"
    })
  }

  # Default origin is the first one in the list
  default_origin_id = length(var.origins) > 0 ? local.origin_configs[var.origins[0].name].origin_id : null

  # Generate tags for all resources
  tags = merge(var.tags, {
    Project      = var.project
    Environment  = var.env
    Distribution = var.name
    ManagedBy    = "meroku"
  })
}

# ============================================================================
# CloudFront Distribution
# ============================================================================

resource "aws_cloudfront_distribution" "main" {
  enabled             = true
  is_ipv6_enabled     = true
  comment             = "${var.project}-${var.env}-${var.name} CloudFront distribution"
  default_root_object = var.default_root_object
  price_class         = var.price_class
  http_version        = "http2and3"

  # Custom domain aliases (including wildcards like *.app.domain.com)
  aliases = var.domain_aliases

  # Origin configurations
  dynamic "origin" {
    for_each = local.origin_configs
    content {
      domain_name = origin.value.domain_name
      origin_id   = origin.value.origin_id
      origin_path = lookup(origin.value, "origin_path", "")

      # Custom origin config for ALB, Amplify, or custom URLs
      dynamic "custom_origin_config" {
        for_each = origin.value.type != "s3" ? [1] : []
        content {
          http_port                = lookup(origin.value, "http_port", 80)
          https_port               = lookup(origin.value, "https_port", 443)
          origin_protocol_policy   = lookup(origin.value, "protocol_policy", "https-only")
          origin_ssl_protocols     = lookup(origin.value, "ssl_protocols", ["TLSv1.2"])
          origin_keepalive_timeout = lookup(origin.value, "keepalive_timeout", 60)
          origin_read_timeout      = lookup(origin.value, "read_timeout", 60)
        }
      }

      # S3 origin config with OAC
      dynamic "s3_origin_config" {
        for_each = origin.value.type == "s3" && lookup(origin.value, "use_oac", true) ? [] : (origin.value.type == "s3" ? [1] : [])
        content {
          origin_access_identity = ""
        }
      }

      # Custom headers to pass to origin (e.g., for tenant identification)
      dynamic "custom_header" {
        for_each = lookup(origin.value, "custom_headers", {})
        content {
          name  = custom_header.key
          value = custom_header.value
        }
      }
    }
  }

  # Origin Access Control for S3 origins
  dynamic "origin" {
    for_each = { for k, v in local.origin_configs : k => v if v.type == "s3" && lookup(v, "use_oac", true) }
    content {
      domain_name              = origin.value.domain_name
      origin_id                = origin.value.origin_id
      origin_access_control_id = aws_cloudfront_origin_access_control.s3[origin.key].id
    }
  }

  # Default cache behavior
  default_cache_behavior {
    allowed_methods  = var.default_allowed_methods
    cached_methods   = var.default_cached_methods
    target_origin_id = local.default_origin_id

    # Forward headers configuration
    forwarded_values {
      query_string = var.forward_query_string
      headers      = var.forward_headers

      cookies {
        forward           = var.forward_cookies
        whitelisted_names = var.forward_cookies == "whitelist" ? var.whitelisted_cookies : null
      }
    }

    viewer_protocol_policy = var.viewer_protocol_policy
    min_ttl                = var.min_ttl
    default_ttl            = var.default_ttl
    max_ttl                = var.max_ttl
    compress               = var.compress

    # Lambda@Edge or CloudFront Functions
    dynamic "lambda_function_association" {
      for_each = var.lambda_function_associations
      content {
        event_type   = lambda_function_association.value.event_type
        lambda_arn   = lambda_function_association.value.lambda_arn
        include_body = lookup(lambda_function_association.value, "include_body", false)
      }
    }

    dynamic "function_association" {
      for_each = var.cloudfront_function_associations
      content {
        event_type   = function_association.value.event_type
        function_arn = function_association.value.function_arn
      }
    }
  }

  # Additional cache behaviors for path-based routing
  dynamic "ordered_cache_behavior" {
    for_each = var.cache_behaviors
    content {
      path_pattern     = ordered_cache_behavior.value.path_pattern
      allowed_methods  = lookup(ordered_cache_behavior.value, "allowed_methods", ["GET", "HEAD"])
      cached_methods   = lookup(ordered_cache_behavior.value, "cached_methods", ["GET", "HEAD"])
      target_origin_id = local.origin_configs[ordered_cache_behavior.value.origin_name].origin_id

      forwarded_values {
        query_string = lookup(ordered_cache_behavior.value, "forward_query_string", true)
        headers      = lookup(ordered_cache_behavior.value, "forward_headers", ["Host", "Origin"])

        cookies {
          forward = lookup(ordered_cache_behavior.value, "forward_cookies", "none")
        }
      }

      viewer_protocol_policy = lookup(ordered_cache_behavior.value, "viewer_protocol_policy", "redirect-to-https")
      min_ttl                = lookup(ordered_cache_behavior.value, "min_ttl", 0)
      default_ttl            = lookup(ordered_cache_behavior.value, "default_ttl", 0)
      max_ttl                = lookup(ordered_cache_behavior.value, "max_ttl", 0)
      compress               = lookup(ordered_cache_behavior.value, "compress", true)
    }
  }

  # Custom error responses (for SPA routing)
  dynamic "custom_error_response" {
    for_each = var.custom_error_responses
    content {
      error_code            = custom_error_response.value.error_code
      response_code         = lookup(custom_error_response.value, "response_code", null)
      response_page_path    = lookup(custom_error_response.value, "response_page_path", null)
      error_caching_min_ttl = lookup(custom_error_response.value, "error_caching_min_ttl", 10)
    }
  }

  # SSL certificate configuration
  viewer_certificate {
    acm_certificate_arn            = var.certificate_arn
    ssl_support_method             = var.certificate_arn != null ? "sni-only" : null
    minimum_protocol_version       = var.certificate_arn != null ? "TLSv1.2_2021" : null
    cloudfront_default_certificate = var.certificate_arn == null
  }

  # Geo restrictions
  restrictions {
    geo_restriction {
      restriction_type = var.geo_restriction_type
      locations        = var.geo_restriction_locations
    }
  }

  # WAF association
  web_acl_id = var.web_acl_id

  # Logging configuration
  dynamic "logging_config" {
    for_each = var.logging_bucket != null ? [1] : []
    content {
      include_cookies = var.logging_include_cookies
      bucket          = var.logging_bucket
      prefix          = var.logging_prefix != null ? var.logging_prefix : "${var.project}/${var.env}/"
    }
  }

  tags = local.tags

  depends_on = [aws_cloudfront_origin_access_control.s3]
}

# ============================================================================
# Origin Access Control for S3 (modern replacement for OAI)
# ============================================================================

resource "aws_cloudfront_origin_access_control" "s3" {
  for_each = { for k, v in local.origin_configs : k => v if v.type == "s3" && lookup(v, "use_oac", true) }

  name                              = "${var.project}-${var.env}-${each.key}-oac"
  description                       = "OAC for ${each.key} S3 origin"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

# ============================================================================
# Route53 Records for CloudFront
# ============================================================================

resource "aws_route53_record" "cloudfront" {
  for_each = var.create_dns_records ? toset(var.domain_aliases) : toset([])

  zone_id = var.zone_id
  name    = each.value
  type    = "A"

  alias {
    name                   = aws_cloudfront_distribution.main.domain_name
    zone_id                = aws_cloudfront_distribution.main.hosted_zone_id
    evaluate_target_health = false
  }
}

resource "aws_route53_record" "cloudfront_ipv6" {
  for_each = var.create_dns_records ? toset(var.domain_aliases) : toset([])

  zone_id = var.zone_id
  name    = each.value
  type    = "AAAA"

  alias {
    name                   = aws_cloudfront_distribution.main.domain_name
    zone_id                = aws_cloudfront_distribution.main.hosted_zone_id
    evaluate_target_health = false
  }
}
