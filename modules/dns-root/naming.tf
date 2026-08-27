# The one capped AWS name this module builds. See modules/naming.
#
# An IAM role name caps at 64 and this one is derived from a domain, which has
# no length bound of its own: a 50-character domain plus "-dns-delegation"
# overflows. There is exactly one of these per domain per account.
#
# This module has no project or env — it is the root DNS zone, which sits above
# both — so those two slots carry the literal "dns" and "delegation" and `parts`
# carries the domain. That keeps the domain leading the name in every form,
# which is what a human is scanning for, and leaves form 2 reading
# "dns-example-com-delegation" rather than something with a hole in it.
module "naming" {
  source  = "../naming"
  project = "dns"
  env     = "delegation"

  requests = {
    delegation_role = {
      legacy = "${replace(var.domain_name, ".", "-")}-dns-delegation"
      parts  = [replace(var.domain_name, ".", "-")]
      limit  = 64
    }
  }
}
