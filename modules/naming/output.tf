output "names" {
  description = "Request key => the AWS name to use. Guaranteed within the caller's limit."
  value       = local.names
}

output "forms" {
  description = <<-EOT
    Request key => which form the name landed on: 1 legacy, 2 undecorated,
    3 digest. A key that reads 1 is byte-identical to what the resource is
    called today, so a plan showing no change for it is the expected result.
  EOT
  value       = local.forms
}

output "collisions" {
  description = <<-EOT
    Names this module produced more than once, empty when all are distinct.

    Within a single form the request key makes names unique, but the cascade
    lets forms MIX — a short resource keeps form 1 while a long one moves to
    form 2 — and a `parts` list that spells out another request's decorated form
    can then land on a name already taken. Callers should assert this is empty
    in a precondition; AWS would otherwise reject the duplicate partway through
    an apply, naming neither resource.
  EOT
  value = [
    for n in distinct(values(local.names)) : n
    if length([for m in values(local.names) : m if m == n]) > 1
  ]
}
