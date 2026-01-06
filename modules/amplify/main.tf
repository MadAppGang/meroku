# Fetch shared GitHub token from SSM using predefined path
data "aws_ssm_parameter" "github_token" {
  name = "/${var.project}/${var.env}/github/amplify-token"
}

locals {
  # Construct domain for each app based on subdomain_prefix or custom_domain
  app_domains = {
    for app in var.amplify_apps : app.name => (
      # If custom_domain is set, use it directly (manual override)
      app.custom_domain != null && app.custom_domain != ""
      ? app.custom_domain
      # If subdomain_prefix is set, auto-construct domain
      : app.subdomain_prefix != null && app.subdomain_prefix != "" && var.base_domain != ""
      ? (var.add_env_domain_prefix && var.env != "prod"
        ? "${app.subdomain_prefix}.${var.env}.${var.base_domain}" # e.g., app.dev.example.com
        : "${app.subdomain_prefix}.${var.base_domain}"            # e.g., app.example.com
      )
      : "" # No domain configured
    )
  }

  # Flatten apps and branches for easier resource creation
  app_branches = flatten([
    for app in var.amplify_apps : [
      for branch in app.branches : {
        app_name = app.name
        app      = app
        branch   = branch
      }
    ]
  ])

  # Calculate branch-specific subdomain mappings
  branch_subdomain_mappings = flatten([
    for app in var.amplify_apps : [
      for branch in app.branches : [
        for subdomain in branch.custom_subdomains : {
          app_name    = app.name
          subdomain   = subdomain
          branch_name = branch.name
          is_root     = false
        }
      ] if length(branch.custom_subdomains) > 0
    ] if local.app_domains[app.name] != ""
  ])

  # Calculate root domain mappings (maps to first PRODUCTION branch or first branch)
  # Always create root mapping if domain is configured
  root_domain_mappings = flatten([
    for app in var.amplify_apps : [
      {
        app_name  = app.name
        subdomain = ""
        branch_name = app.branches[
          coalesce(
            try(index(app.branches[*].stage, "PRODUCTION"), 0),
            0
          )
        ].name
        is_root = true
      }
    ] if local.app_domains[app.name] != ""
  ])

  # Combine all subdomain mappings
  all_subdomain_mappings = concat(
    local.branch_subdomain_mappings,
    local.root_domain_mappings
  )
}

resource "aws_amplify_app" "apps" {
  for_each = { for app in var.amplify_apps : app.name => app }

  name                        = each.value.name
  repository                  = each.value.github_repository
  oauth_token                 = data.aws_ssm_parameter.github_token.value
  platform                    = "WEB"
  enable_branch_auto_build    = true
  enable_auto_branch_creation = false

  # Build spec is not set here - users should provide amplify.yml in their repository
  # for custom build configurations. Amplify will auto-detect the framework and
  # use appropriate default build settings if amplify.yml is not present.

  # Default redirect for SPAs
  custom_rule {
    source = "/<*>"
    target = "/index.html"
    status = "404-200"
  }

  # Environment variables at app level
  environment_variables = each.value.environment_variables

  tags = {
    Name        = "${var.project}-amplify-${each.value.name}-${var.env}"
    Environment = var.env
    Project     = var.project
    Application = each.value.name
    ManagedBy   = "meroku"
  }
}

resource "aws_amplify_branch" "branches" {
  for_each = {
    for branch in local.app_branches : "${branch.app_name}-${branch.branch.name}" => branch
  }

  app_id      = aws_amplify_app.apps[each.value.app_name].id
  branch_name = each.value.branch.name

  display_name                = each.value.branch.name
  enable_notification         = false
  enable_auto_build           = each.value.branch.enable_auto_build
  enable_pull_request_preview = each.value.branch.enable_pull_request_preview

  stage = each.value.branch.stage

  # Branch-specific environment variables
  environment_variables = each.value.branch.environment_variables

  tags = {
    Name        = "${var.project}-amplify-branch-${each.value.app_name}-${each.value.branch.name}-${var.env}"
    Environment = var.env
    Project     = var.project
    Application = each.value.app_name
    Branch      = each.value.branch.name
    Stage       = each.value.branch.stage
    ManagedBy   = "meroku"
  }
}

# Custom domain configuration
resource "aws_amplify_domain_association" "domains" {
  for_each = {
    for app in var.amplify_apps : app.name => app
    if local.app_domains[app.name] != ""
  }

  app_id      = aws_amplify_app.apps[each.key].id
  domain_name = local.app_domains[each.key]

  # Don't wait for verification - allows faster apply and avoids timeouts
  # Verification can complete asynchronously after apply
  wait_for_verification = false

  # Configure all subdomain mappings (legacy app-level + branch-specific + root domain)
  dynamic "sub_domain" {
    for_each = [
      for mapping in local.all_subdomain_mappings : mapping
      if mapping.app_name == each.key
    ]
    content {
      branch_name = sub_domain.value.branch_name
      prefix      = sub_domain.value.subdomain
    }
  }

  # Handle domain changes gracefully:
  # - create_before_destroy prevents "already associated" errors during domain changes
  # - ignore_changes on certificate_settings prevents drift from Amplify-managed certs
  lifecycle {
    create_before_destroy = true
  }

  # Branches must exist before domain association can map subdomains to them
  depends_on = [aws_amplify_branch.branches]
}

# -----------------------------------------------------------------------------
# Route53 DNS Records for Amplify Domain Verification
# These records are required for ACM certificate validation and subdomain routing
# -----------------------------------------------------------------------------

locals {
  # Static keys for apps that need certificate validation records
  # Keys are derived from input configuration (known at plan time), not resource outputs
  apps_needing_dns = {
    for app in var.amplify_apps : app.name => {
      domain_name = local.app_domains[app.name]
    }
    if var.zone_id != "" && local.app_domains[app.name] != ""
  }

  # Static keys for subdomain records - computed from input configuration
  # Maps each app to its configured subdomain mappings (known at plan time)
  # Key format: "${app_name}" for root domain, "${app_name}-${subdomain}" for subdomains
  subdomain_keys = {
    for mapping in local.all_subdomain_mappings : (
      mapping.subdomain == "" ? mapping.app_name : "${mapping.app_name}-${mapping.subdomain}"
    ) => {
      app_name    = mapping.app_name
      domain_name = local.app_domains[mapping.app_name]
      prefix      = mapping.subdomain
      record_name = mapping.subdomain != "" ? "${mapping.subdomain}.${local.app_domains[mapping.app_name]}" : local.app_domains[mapping.app_name]
    }
    if var.zone_id != "" && local.app_domains[mapping.app_name] != ""
  }
}

# ACM Certificate Validation CNAME Records
# These records prove domain ownership to AWS Certificate Manager
# Uses static keys derived from input config to avoid for_each unknown keys issue
resource "aws_route53_record" "amplify_cert_validation" {
  for_each = local.apps_needing_dns

  zone_id = var.zone_id
  # Extract the record name from certificate_verification_dns_record (format: "_hash.domain. CNAME _hash.acm-validations.aws.")
  name = trimspace(split(" CNAME ", aws_amplify_domain_association.domains[each.key].certificate_verification_dns_record)[0])
  type = "CNAME"
  ttl  = 60
  # Extract the record value (everything after " CNAME ")
  records         = [trimspace(split(" CNAME ", aws_amplify_domain_association.domains[each.key].certificate_verification_dns_record)[1])]
  allow_overwrite = true

  # Depends on domain association to ensure cert validation record is available
  depends_on = [aws_amplify_domain_association.domains]
}

# Subdomain CNAME Records pointing to CloudFront
# These records route traffic from custom domains to Amplify's CloudFront distribution
# Uses static keys derived from input config to avoid for_each unknown keys issue
resource "aws_route53_record" "amplify_subdomain" {
  for_each = local.subdomain_keys

  zone_id = var.zone_id
  name    = each.value.record_name
  type    = "CNAME"
  ttl     = 300
  # Dynamically look up the CloudFront domain from the domain association's sub_domain list
  records = [
    trimspace(element(split(" CNAME ", [
      for sub in aws_amplify_domain_association.domains[each.value.app_name].sub_domain : sub.dns_record
      if sub.prefix == each.value.prefix
    ][0]), 1))
  ]
  allow_overwrite = true

  # Depends on domain association to ensure subdomain info is available
  depends_on = [aws_amplify_domain_association.domains]
}