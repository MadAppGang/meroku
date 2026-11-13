# Exact role grants and isolated folder variables.
mock_provider "aws" {}

override_data {
  target = data.aws_caller_identity.current
  values = { account_id = "000000000000" }
}

variables {
  project             = "myapp"
  env                 = "dev"
  backend_bucket_name = "myapp-backend-dev"
  backend_bucket_arn  = "arn:aws:s3:::myapp-backend-dev"
  workloads           = {}
}

override_resource {
  target = aws_s3_bucket.data
  values = {
    arn = "arn:aws:s3:::myapp-data-dev-000000000000"
    id  = "myapp-data-dev-000000000000"
  }
}

run "backend_gets_only_its_data_folder" {
  # The bucket ARN is computed, so inspect the real policy expression after a mocked apply.
  command = apply

  variables {
    workloads = {
      backend       = { task_role_name = "myapp_backend_task_dev" }
      api           = { task_role_name = "myapp_service_api_task_dev" }
      "api-billing" = { task_role_name = "myapp_service_api-billing_task_dev" }
    }
  }

  assert {
    condition = toset(flatten([
      for st in jsondecode(output.policies["backend"]).Statement : [
        for a in try(tolist(st.Action), [tostring(st.Action)]) : [
          for r in try(tolist(st.Resource), [tostring(st.Resource)]) : "${st.Effect}|${a}|${r}"
        ]
      ]
      ])) == toset([
      "Allow|s3:GetObject|arn:aws:s3:::myapp-data-dev-000000000000/backend/*",
      "Allow|s3:PutObject|arn:aws:s3:::myapp-data-dev-000000000000/backend/*",
      "Allow|s3:DeleteObject|arn:aws:s3:::myapp-data-dev-000000000000/backend/*",
      "Allow|s3:AbortMultipartUpload|arn:aws:s3:::myapp-data-dev-000000000000/backend/*",
      "Allow|s3:ListBucket|arn:aws:s3:::myapp-data-dev-000000000000"
    ])
    error_message = "backend must have exactly the documented object and bucket action/resource pairs, on its task role only."
  }

  assert {
    condition     = aws_iam_role_policy.workload["backend"].role == "myapp_backend_task_dev"
    error_message = "The backend's private data-folder grant must be on its task role."
  }

  assert {
    condition = anytrue([
      for st in jsondecode(aws_s3_bucket_policy.data.policy).Statement :
      st.Effect == "Deny" &&
      length(setintersection(toset(try(tolist(st.Action), [tostring(st.Action)])), toset(["s3:*", "*"]))) > 0 &&
      toset(try(tolist(st.Resource), [tostring(st.Resource)])) == toset([
        "arn:aws:s3:::myapp-data-dev-000000000000",
        "arn:aws:s3:::myapp-data-dev-000000000000/*"
      ]) &&
      try(tostring(st.Condition.Bool["aws:SecureTransport"]) == "false", false) &&
      (try(st.Principal == "*", false) || try(toset(try(tolist(st.Principal.AWS), [tostring(st.Principal.AWS)])) == toset(["*"]), false)) &&
      try(toset(keys(st.Condition)) == toset(["Bool"]), false) &&
      try(toset(keys(st.Condition.Bool)) == toset(["aws:SecureTransport"]), false)
    ])
    error_message = "The bucket resource policy must deny non-TLS requests from every principal to the bucket and its objects."
  }
}

run "shared_access_and_sibling_prefixes_stay_isolated" {
  command = apply

  variables {
    workloads = {
      backend       = { task_role_name = "myapp_backend_task_dev" }
      api           = { task_role_name = "myapp_service_api_task_dev" }
      "api-billing" = { task_role_name = "myapp_service_api-billing_task_dev" }
    }
    shared = [
      { name = "uploads", read_write = ["api"], read = ["api-billing"] },
      { name = "reference", read = ["api"], read_write = ["api-billing"] }
    ]
  }

  assert {
    condition = toset(flatten([
      for st in jsondecode(output.policies["api"]).Statement : [
        for a in try(tolist(st.Action), [tostring(st.Action)]) : [
          for r in try(tolist(st.Resource), [tostring(st.Resource)]) : "${st.Effect}|${a}|${r}"
        ]
      ]
      ])) == toset([
      "Allow|s3:GetObject|arn:aws:s3:::myapp-data-dev-000000000000/api/*",
      "Allow|s3:GetObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/uploads/*",
      "Allow|s3:PutObject|arn:aws:s3:::myapp-data-dev-000000000000/api/*",
      "Allow|s3:PutObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/uploads/*",
      "Allow|s3:DeleteObject|arn:aws:s3:::myapp-data-dev-000000000000/api/*",
      "Allow|s3:DeleteObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/uploads/*",
      "Allow|s3:AbortMultipartUpload|arn:aws:s3:::myapp-data-dev-000000000000/api/*",
      "Allow|s3:AbortMultipartUpload|arn:aws:s3:::myapp-data-dev-000000000000/shared/uploads/*",
      "Allow|s3:GetObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/reference/*",
      "Allow|s3:ListBucket|arn:aws:s3:::myapp-data-dev-000000000000",
      "Allow|s3:GetObject|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:PutObject|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:DeleteObject|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:AbortMultipartUpload|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:ListMultipartUploadParts|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:PutObjectAcl|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:ListBucket|arn:aws:s3:::myapp-backend-dev",
      "Allow|s3:GetBucketLocation|arn:aws:s3:::myapp-backend-dev"
    ])
    error_message = "api must have exactly the documented object and bucket action/resource pairs, on its task role only."
  }

  assert {
    condition = toset(flatten([
      for st in jsondecode(output.policies["api-billing"]).Statement : [
        for a in try(tolist(st.Action), [tostring(st.Action)]) : [
          for r in try(tolist(st.Resource), [tostring(st.Resource)]) : "${st.Effect}|${a}|${r}"
        ]
      ]
      ])) == toset([
      "Allow|s3:GetObject|arn:aws:s3:::myapp-data-dev-000000000000/api-billing/*",
      "Allow|s3:GetObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/reference/*",
      "Allow|s3:PutObject|arn:aws:s3:::myapp-data-dev-000000000000/api-billing/*",
      "Allow|s3:PutObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/reference/*",
      "Allow|s3:DeleteObject|arn:aws:s3:::myapp-data-dev-000000000000/api-billing/*",
      "Allow|s3:DeleteObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/reference/*",
      "Allow|s3:AbortMultipartUpload|arn:aws:s3:::myapp-data-dev-000000000000/api-billing/*",
      "Allow|s3:AbortMultipartUpload|arn:aws:s3:::myapp-data-dev-000000000000/shared/reference/*",
      "Allow|s3:GetObject|arn:aws:s3:::myapp-data-dev-000000000000/shared/uploads/*",
      "Allow|s3:ListBucket|arn:aws:s3:::myapp-data-dev-000000000000",
      "Allow|s3:GetObject|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:PutObject|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:DeleteObject|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:AbortMultipartUpload|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:ListMultipartUploadParts|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:PutObjectAcl|arn:aws:s3:::myapp-backend-dev/*",
      "Allow|s3:ListBucket|arn:aws:s3:::myapp-backend-dev",
      "Allow|s3:GetBucketLocation|arn:aws:s3:::myapp-backend-dev"
    ])
    error_message = "api-billing must have exactly the documented object and bucket action/resource pairs, on its task role only."
  }

  assert {
    condition = alltrue([
      for w, prefixes in {
        backend       = ["backend/*"]
        api           = ["api/*", "shared/reference/*", "shared/uploads/*"]
        "api-billing" = ["api-billing/*", "shared/reference/*", "shared/uploads/*"]
      } :
      sort(one([
        for st in jsondecode(output.policies[w]).Statement : st
        if contains(try(tolist(st.Action), [tostring(st.Action)]), "s3:ListBucket") &&
        contains(try(tolist(st.Resource), [tostring(st.Resource)]), "arn:aws:s3:::myapp-data-dev-000000000000")
      ]).Condition.StringLike["s3:prefix"]) == sort(prefixes)
    ])
    error_message = "Data-bucket listing must use StringLike with exactly the workload's own /* and authorized shared-folder /* patterns; api* would expose api-billing/."
  }

  assert {
    condition = alltrue([
      for policy in values(output.policies) : alltrue([
        for st in jsondecode(policy).Statement :
        contains(keys(st), "Condition") ? (
          toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["s3:ListBucket"]) &&
          toset(try(tolist(st.Resource), [tostring(st.Resource)])) == toset(["arn:aws:s3:::myapp-data-dev-000000000000"]) &&
          toset(keys(st.Condition)) == toset(["StringLike"]) &&
          try(toset(keys(st.Condition.StringLike)) == toset(["s3:prefix"]), false)
        ) : true
      ])
    ])
    error_message = "Only the data-bucket ListBucket statement may carry the prefix condition; object access and backend-bucket access must not inherit it or unrelated restrictions."
  }

  assert {
    condition = alltrue([
      for w, p in output.policies : alltrue([
        for st in jsondecode(p).Statement : alltrue([
          for a in try(tolist(st.Action), [tostring(st.Action)]) :
          a != "s3:*" && a != "s3:DeleteObjectVersion" &&
          !startswith(a, "s3:PutBucket") && !startswith(a, "s3:DeleteBucket")
        ])
      ])
    ])
    error_message = "Workload policies must never grant bucket administration or permanent version deletion."
  }

  assert {
    condition = {
      for w, p in aws_iam_role_policy.workload : w => p.role
      } == {
      backend       = "myapp_backend_task_dev"
      api           = "myapp_service_api_task_dev"
      "api-billing" = "myapp_service_api-billing_task_dev"
    }
    error_message = "All bucket grants must attach to the supplied task roles, not execution roles."
  }
}

run "bucket_variables_keep_yaml_bucket_ownership" {
  command = plan

  variables {
    workloads = {
      backend       = { task_role_name = "myapp_backend_task_dev" }
      api           = { task_role_name = "myapp_service_api_task_dev" }
      "api-billing" = { task_role_name = "myapp_service_api-billing_task_dev" }
    }
    own_backend_bucket_env = ["api-billing"]
    shared = [
      { name = "uploads", read_write = ["api"], read = ["api-billing"] },
      { name = "reference", read = ["api"] }
    ]
  }

  assert {
    condition = { for w, entries in output.environment : w => { for e in entries : e.name => e.value } } == {
      backend = {
        DATA_BUCKET_NAME = "myapp-data-dev-000000000000"
        DATA_BUCKET_PATH = "backend/"
      }
      api = {
        DATA_BUCKET_NAME             = "myapp-data-dev-000000000000"
        DATA_BUCKET_PATH             = "api/"
        DATA_BUCKET_SHARED_REFERENCE = "shared/reference/"
        DATA_BUCKET_SHARED_UPLOADS   = "shared/uploads/"
        AWS_S3_BUCKET                = "myapp-backend-dev"
      }
      "api-billing" = {
        DATA_BUCKET_NAME           = "myapp-data-dev-000000000000"
        DATA_BUCKET_PATH           = "api-billing/"
        DATA_BUCKET_SHARED_UPLOADS = "shared/uploads/"
      }
    }
    error_message = "Folder variables must be per workload; backend and a workload owning its YAML AWS_S3_BUCKET must not receive another AWS_S3_BUCKET."
  }
}
