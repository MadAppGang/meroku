# One inline policy on every workload's task role: its own folder of the data
# bucket, the shared folders it is listed on and, for every workload except the
# backend, object-level read-write on the backend bucket.
#
# Inline rather than managed: a role holds at most 10 managed policies and the
# backend's task role is already at that quota. The backend's own access to the
# backend bucket is its existing managed attachment in modules/workloads, which is
# why the backend gets no backend-bucket statements here.

locals {
  # Sorted, so reordering the YAML changes no policy and no task definition.
  shared_names   = sort([for s in var.shared : s.name])
  shared_by_name = { for s in var.shared : s.name => s }

  read_write_folders = { for w in keys(var.workloads) : w => [
    for f in local.shared_names : f if contains(local.shared_by_name[f].read_write, w)
  ] }

  # A folder listed in both lists for one workload is read-write.
  read_folders = { for w in keys(var.workloads) : w => [
    for f in local.shared_names : f if contains(local.shared_by_name[f].read, w) && !contains(local.shared_by_name[f].read_write, w)
  ] }

  data_bucket_arn = aws_s3_bucket.data.arn

  # Never s3:DeleteObjectVersion: a deleted object stays a noncurrent version for
  # the 30 days of the lifecycle rule, and no task can remove that copy.
  data_object_actions = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload"]

  backend_bucket_object_actions = [
    "s3:GetObject",
    "s3:PutObject",
    "s3:DeleteObject",
    "s3:AbortMultipartUpload",
    "s3:ListMultipartUploadParts",
    # Has an effect only when the backend bucket is public, which is what a
    # public bucket exists for.
    "s3:PutObjectAcl",
  ]
  backend_bucket_actions = ["s3:ListBucket", "s3:GetBucketLocation"]

  workload_policy_statements = { for w in keys(var.workloads) : w => concat(
    [
      {
        Sid    = "DataBucketReadWrite"
        Effect = "Allow"
        Action = local.data_object_actions
        Resource = concat(
          ["${local.data_bucket_arn}/${w}/*"],
          [for f in local.read_write_folders[w] : "${local.data_bucket_arn}/shared/${f}/*"],
        )
      },
    ],
    length(local.read_folders[w]) > 0 ? [
      {
        Sid      = "DataBucketReadOnly"
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = [for f in local.read_folders[w] : "${local.data_bucket_arn}/shared/${f}/*"]
      },
    ] : [],
    [
      {
        Sid      = "DataBucketList"
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = [local.data_bucket_arn]
        # StringLike, because StringEquals compares the * literally. The / before
        # the * is required: "api*" would also let api list api-billing/.
        Condition = {
          StringLike = {
            "s3:prefix" = concat(
              ["${w}/*"],
              [for f in concat(local.read_write_folders[w], local.read_folders[w]) : "shared/${f}/*"],
            )
          }
        }
      },
    ],
    w == "backend" ? [] : [
      {
        Sid      = "BackendBucketObjects"
        Effect   = "Allow"
        Action   = local.backend_bucket_object_actions
        Resource = ["${var.backend_bucket_arn}/*"]
      },
      {
        Sid      = "BackendBucketList"
        Effect   = "Allow"
        Action   = local.backend_bucket_actions
        Resource = [var.backend_bucket_arn]
      },
    ],
  ) }
}

resource "aws_iam_role_policy" "workload" {
  for_each = var.workloads

  name = "${var.project}_data_bucket_${each.key}_${var.env}"
  role = each.value.task_role_name
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = local.workload_policy_statements[each.key]
  })
}
