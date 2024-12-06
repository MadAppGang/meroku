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