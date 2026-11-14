# The environment's private data bucket. Every workload gets its own folder,
# <name>/, and the shared folders, shared/<folder>/, it is listed on.
#
# Nothing here is configurable from the YAML except the shared folders: the
# bucket is private, encrypted and versioned in every environment.

resource "aws_s3_bucket" "data" {
  bucket = module.naming.names["data_bucket"]

  # Fixed. Destroying an environment fails while the bucket holds any object
  # version, so data is never deleted as a side effect of a destroy; empty every
  # version first. See ai_docs/DATA_BUCKET.md.
  force_destroy = false

  tags = {
    Name        = module.naming.names["data_bucket"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_s3_bucket_public_access_block" "data" {
  bucket = aws_s3_bucket.data.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ACLs are disabled: the bucket owns every object, and a request that carries
# an ACL header is refused.
resource "aws_s3_bucket_ownership_controls" "data" {
  bucket = aws_s3_bucket.data.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "data" {
  bucket = aws_s3_bucket.data.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_versioning" "data" {
  bucket = aws_s3_bucket.data.id

  versioning_configuration {
    status = "Enabled"
  }
}

# A deleted or overwritten object stays recoverable for 30 days. No workload is
# granted s3:DeleteObjectVersion, so this window is the protection.
resource "aws_s3_bucket_lifecycle_configuration" "data" {
  bucket = aws_s3_bucket.data.id

  rule {
    id     = "noncurrent-versions-and-uploads"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = 30
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  # A lifecycle rule on noncurrent versions needs versioning in place first.
  depends_on = [aws_s3_bucket_versioning.data]
}

resource "aws_s3_bucket_policy" "data" {
  bucket = aws_s3_bucket.data.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "DenyInsecureTransport"
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource  = [aws_s3_bucket.data.arn, "${aws_s3_bucket.data.arn}/*"]
      Condition = { Bool = { "aws:SecureTransport" = "false" } }
    }]
  })

  # Applied after the public access block so the two never race on a new bucket.
  depends_on = [aws_s3_bucket_public_access_block.data]
}
