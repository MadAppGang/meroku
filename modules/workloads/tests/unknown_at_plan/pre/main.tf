# Stands in for a custom/pre module: one resource created in the same apply,
# and a backend_policies output in the documented {actions, resources} shape
# whose resources name it.

resource "terraform_data" "key" {
  input = "pending"
}

output "backend_policies" {
  value = [
    { actions = ["kms:Decrypt"], resources = [terraform_data.key.id] },
  ]
}
