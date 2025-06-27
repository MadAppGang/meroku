locals {
  bucket_names = { for bucket in var.buckets : bucket.name => bucket }
}

resource "aws_s3_bucket" "this" {
  for_each = local.bucket_names

  bucket = "${var.project}-${each.key}-${var.env}"

  # Add tags for better resource management
  tags = {
    Name        = "${var.project}-${each.key}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Configure versioning for the S3 bucket
resource "aws_s3_bucket_versioning" "this" {
  for_each = local.bucket_names

  bucket = aws_s3_bucket.this[each.key].id

  versioning_configuration {
    status = each.value.versioning == false ? "Disabled" : "Enabled"
  }
}

# Configure CORS for buckets that have CORS rules defined
resource "aws_s3_bucket_cors_configuration" "this" {
  for_each = { for name, bucket in local.bucket_names : name => bucket if length(bucket.cors_rules) > 0 }

  bucket = aws_s3_bucket.this[each.key].id

  dynamic "cors_rule" {
    for_each = each.value.cors_rules

    content {
      allowed_headers = cors_rule.value.allowed_headers
      allowed_methods = cors_rule.value.allowed_methods
      allowed_origins = cors_rule.value.allowed_origins
      expose_headers  = cors_rule.value.expose_headers
      max_age_seconds = cors_rule.value.max_age_seconds
    }
  }
}

# Block public access by default
resource "aws_s3_bucket_public_access_block" "this" {
  for_each = { for name, bucket in local.bucket_names : name => bucket if !bucket.public }

  bucket = aws_s3_bucket.this[each.key].id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# Configure public access for buckets marked as public
resource "aws_s3_bucket_public_access_block" "public" {
  for_each = { for name, bucket in local.bucket_names : name => bucket if bucket.public }

  bucket = aws_s3_bucket.this[each.key].id

  block_public_acls       = false
  block_public_policy     = false
  ignore_public_acls      = false
  restrict_public_buckets = false
}

# Add bucket policy for public buckets
resource "aws_s3_bucket_policy" "public" {
  for_each = { for name, bucket in local.bucket_names : name => bucket if bucket.public }

  bucket = aws_s3_bucket.this[each.key].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "PublicReadGetObject"
        Effect    = "Allow"
        Principal = "*"
        Action    = "s3:GetObject"
        Resource  = "${aws_s3_bucket.this[each.key].arn}/*"
      },
    ]
  })

  depends_on = [aws_s3_bucket_public_access_block.public]
}
