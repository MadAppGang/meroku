# The public key of every asymmetric (signing) key, read from KMS by ARN. HMAC
# keys have no public key and are never passed in.
#
# This read lives in its own module so that it refers to an input variable, not
# to aws_kms_key.key. Terraform records a data source's dependency on a managed
# resource for the whole resource block, not per instance: with the read beside
# the keys, adding or deleting any one key deferred every key's read to apply,
# left every public key unknown at plan, and so replaced the task definition of
# every connected workload. Here a key that already exists has a known ARN, and
# its public key is read at plan.
data "aws_kms_public_key" "key" {
  for_each = var.key_arns

  key_id = each.value
}
