output "amplify_apps" {
  description = "Map of Amplify app details including all branches"
  value = {
    for app_name, app in aws_amplify_app.apps : app_name => {
      id             = app.id
      arn            = app.arn
      default_domain = app.default_domain
      branches = {
        for branch_key, branch in aws_amplify_branch.branches : branch.branch_name => {
          id          = branch.id
          branch_name = branch.branch_name
          stage       = branch.stage
          url         = "https://${branch.branch_name}.${app.default_domain}"
        } if startswith(branch_key, "${app_name}-")
      }
      primary_branch = try(
        [for b in local.app_branches : b.branch.name if b.app_name == app_name && b.branch.stage == "PRODUCTION"][0],
        [for b in local.app_branches : b.branch.name if b.app_name == app_name][0]
      )
      app_url = try(
        aws_amplify_domain_association.domains[app_name] != null ? 
          "https://${aws_amplify_domain_association.domains[app_name].domain_name}" : 
          "https://${try([for b in local.app_branches : b.branch.name if b.app_name == app_name && b.branch.stage == "PRODUCTION"][0], [for b in local.app_branches : b.branch.name if b.app_name == app_name][0])}.${app.default_domain}",
        "https://${try([for b in local.app_branches : b.branch.name if b.app_name == app_name][0])}.${app.default_domain}"
      )
      custom_domain = try(aws_amplify_domain_association.domains[app_name].domain_name, null)
    }
  }
}

output "app_ids" {
  description = "Map of Amplify app IDs"
  value = { for app_name, app in aws_amplify_app.apps : app_name => app.id }
}

output "app_arns" {
  description = "Map of Amplify app ARNs"
  value = { for app_name, app in aws_amplify_app.apps : app_name => app.arn }
}

output "default_domains" {
  description = "Map of Amplify app default domains"
  value = { for app_name, app in aws_amplify_app.apps : app_name => app.default_domain }
}

output "app_urls" {
  description = "Map of primary URLs to access the applications"
  value = {
    for app_name, app in aws_amplify_app.apps : app_name => try(
      aws_amplify_domain_association.domains[app_name] != null ? 
        "https://${aws_amplify_domain_association.domains[app_name].domain_name}" : 
        "https://${try([for b in local.app_branches : b.branch.name if b.app_name == app_name && b.branch.stage == "PRODUCTION"][0], [for b in local.app_branches : b.branch.name if b.app_name == app_name][0])}.${app.default_domain}",
      "https://${try([for b in local.app_branches : b.branch.name if b.app_name == app_name][0])}.${app.default_domain}"
    )
  }
}

output "branch_urls" {
  description = "Map of all branch URLs for each app"
  value = {
    for app_name, app in aws_amplify_app.apps : app_name => {
      for branch_key, branch in aws_amplify_branch.branches : 
        branch.branch_name => "https://${branch.branch_name}.${app.default_domain}"
        if startswith(branch_key, "${app_name}-")
    }
  }
}