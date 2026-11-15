output "public_keys" {
  description = "The PEM-encoded public key of every key of var.key_arns, keyed by key name."
  value       = { for k, d in data.aws_kms_public_key.key : k => d.public_key_pem }
}
