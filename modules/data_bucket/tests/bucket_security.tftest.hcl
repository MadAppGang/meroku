# Privacy, recovery and account-scoped naming.
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

run "secure_bucket_has_recovery_and_transport_guards" {
  command = plan

  assert {
    condition     = aws_s3_bucket.data.bucket == "myapp-data-dev-000000000000" && aws_s3_bucket.data.force_destroy == false
    error_message = "The private data bucket must exist even without workloads, with an account suffix and force_destroy=false."
  }

  assert {
    condition = (
      aws_s3_bucket_public_access_block.data.block_public_acls &&
      aws_s3_bucket_public_access_block.data.block_public_policy &&
      aws_s3_bucket_public_access_block.data.ignore_public_acls &&
      aws_s3_bucket_public_access_block.data.restrict_public_buckets
    )
    error_message = "All four public-access protections must be enabled."
  }

  assert {
    condition     = one(aws_s3_bucket_ownership_controls.data.rule).object_ownership == "BucketOwnerEnforced"
    error_message = "ACLs must be disabled with BucketOwnerEnforced."
  }

  assert {
    condition     = one(one(aws_s3_bucket_server_side_encryption_configuration.data.rule).apply_server_side_encryption_by_default).sse_algorithm == "AES256"
    error_message = "The data bucket must use SSE-S3 encryption."
  }

  assert {
    condition     = one(aws_s3_bucket_versioning.data.versioning_configuration).status == "Enabled"
    error_message = "Versioning must be enabled on the private data bucket."
  }

  assert {
    condition = anytrue([
      for r in aws_s3_bucket_lifecycle_configuration.data.rule :
      r.status == "Enabled" && try(one(r.noncurrent_version_expiration).noncurrent_days == 30, false)
    ])
    error_message = "An enabled lifecycle rule must expire noncurrent versions after 30 days."
  }

  assert {
    condition = anytrue([
      for r in aws_s3_bucket_lifecycle_configuration.data.rule :
      r.status == "Enabled" && try(one(r.abort_incomplete_multipart_upload).days_after_initiation == 7, false)
    ])
    error_message = "An enabled lifecycle rule must abort incomplete multipart uploads after seven days."
  }
}

run "long_names_keep_the_account_suffix" {
  command = plan

  variables {
    project = "myapp-with-a-very-long-project-name-that-needs-the-naming-cascade"
    env     = "development-environment-with-a-long-name"
  }

  assert {
    condition = (
      length(aws_s3_bucket.data.bucket) <= 63 &&
      endswith(aws_s3_bucket.data.bucket, "-000000000000") &&
      can(regex("^[a-z0-9][a-z0-9-]*[a-z0-9]$", aws_s3_bucket.data.bucket))
    )
    error_message = "Shortening a bucket name must preserve its account suffix and valid S3 characters within 63 characters."
  }
}
