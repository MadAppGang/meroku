# Ordered rewrites, resolved workload targets and app-level variables.
mock_provider "aws" {}

override_data {
  target = data.aws_ssm_parameter.github_token
  values = { value = "fake-key-do-not-use" }
}

variables {
  project = "myapp"
  env     = "dev"
  workload_urls = {
    backend = "https://api.example.com"
    reports = "https://api.example.com/reports"
  }
  amplify_apps = [{
    name              = "web"
    github_repository = "myapp/web"
    branches          = [{ name = "main", stage = "PRODUCTION" }]
    environment_variables = {
      AMPLIFY_MONOREPO_APP_ROOT = "apps/web"
      PUBLIC_API_PATH           = "/api"
    }
    custom_rules = [
      { source = "/api/<*>", workload = "backend", target_path = "/api/<*>" },
      { source = "/reports/<*>", workload = "reports", target_path = "/<*>" },
      { source = "/old-pricing", target = "/pricing", status = "301" },
      { source = "/temporary", target = "/new", status = "302" },
      { source = "/gone", target = "/not-found.html", status = "404-200" }
    ]
  }]
}

run "custom_rules_precede_spa_and_keep_their_targets" {
  command = plan

  assert {
    condition = (
      length(aws_amplify_app.apps["web"].custom_rule) == 6 &&
      [for r in slice(aws_amplify_app.apps["web"].custom_rule, 0, 5) : { source = r.source, target = r.target, status = r.status }] == [
        { source = "/api/<*>", target = "https://api.example.com/api/<*>", status = "200" },
        { source = "/reports/<*>", target = "https://api.example.com/reports/<*>", status = "200" },
        { source = "/old-pricing", target = "/pricing", status = "301" },
        { source = "/temporary", target = "/new", status = "302" },
        { source = "/gone", target = "/not-found.html", status = "404-200" }
      ] &&
      aws_amplify_app.apps["web"].custom_rule[5].target == "/index.html" &&
      aws_amplify_app.apps["web"].custom_rule[5].status == "200"
    )
    error_message = "Amplify is first-match: all custom rules must keep input order before SPA, and workload targets must concatenate the provided base and target_path exactly once."
  }

  assert {
    condition = (
      lookup(aws_amplify_app.apps["web"].environment_variables, "AMPLIFY_MONOREPO_APP_ROOT", "") == "apps/web" &&
      lookup(aws_amplify_app.apps["web"].environment_variables, "PUBLIC_API_PATH", "") == "/api"
    )
    error_message = "App-level environment_variables, including AMPLIFY_MONOREPO_APP_ROOT, must reach the Amplify app, not only its branch."
  }
}

run "spa_disabled_preserves_ordered_custom_rules" {
  command = plan

  variables {
    amplify_apps = [{
      name              = "web"
      github_repository = "myapp/web"
      branches          = [{ name = "main" }]
      spa_mode          = false
      custom_rules = [
        { source = "/legacy", target = "/current", status = "301" },
        { source = "/proxy/<*>", workload = "backend", target_path = "/<*>" }
      ]
    }]
  }

  assert {
    # The contract fixes custom-rule precedence, not non-SPA fallback behavior.
    condition = try(
      [for r in slice(aws_amplify_app.apps["web"].custom_rule, 0, 2) : { source = r.source, target = r.target, status = r.status }] == [
        { source = "/legacy", target = "/current", status = "301" },
        { source = "/proxy/<*>", target = "https://api.example.com/<*>", status = "200" }
      ],
      false
    )
    error_message = "With SPA disabled, custom rules must still lead the rule list in input order with their exact targets and statuses."
  }
}

run "missing_workload_url_fails_the_plan" {
  command = plan

  variables {
    workload_urls = {}
    amplify_apps = [{
      name              = "web"
      github_repository = "myapp/web"
      branches          = [{ name = "main" }]
      custom_rules      = [{ source = "/api/<*>", workload = "missing", target_path = "/api/<*>" }]
    }]
  }

  expect_failures = [aws_amplify_app.apps["web"]]
}
