output "bucket_name" {
  description = "Name of the data bucket."
  value       = aws_s3_bucket.data.bucket
}

output "bucket_arn" {
  description = "ARN of the data bucket."
  value       = aws_s3_bucket.data.arn
}

# Per workload, the variables its container gets. The backend sets AWS_S3_BUCKET
# itself (modules/workloads/env.tf), and a workload whose YAML sets it keeps its
# own value, so neither gets it here.
output "environment" {
  description = "Environment variables for each workload, keyed by workload name."
  value = { for w in keys(var.workloads) : w => concat(
    [
      { name = "DATA_BUCKET_NAME", value = aws_s3_bucket.data.bucket },
      { name = "DATA_BUCKET_PATH", value = "${w}/" },
    ],
    [for f in sort(concat(local.read_write_folders[w], local.read_folders[w])) : {
      name  = "DATA_BUCKET_SHARED_${upper(replace(f, "-", "_"))}"
      value = "shared/${f}/"
    }],
    w == "backend" || contains(var.own_backend_bucket_env, w) ? [] : [
      { name = "AWS_S3_BUCKET", value = var.backend_bucket_name },
    ],
  ) }
}

output "policies" {
  description = "The inline policy document written on each workload's task role, keyed by workload name."
  value       = { for w, p in aws_iam_role_policy.workload : w => p.policy }
}
