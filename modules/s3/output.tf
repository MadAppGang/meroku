output "buckets" {
  description = "Map of created S3 buckets with their ARNs"
  value = {
    for name, bucket in aws_s3_bucket.this : name => {
      arn    = bucket.arn
      id     = bucket.id
      bucket = bucket.bucket
    }
  }
}