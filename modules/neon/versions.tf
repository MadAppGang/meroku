# kislerdm/neon is pinned to one exact release. The provider is archived, and a
# newer release would change the resource schema under existing state, so meroku
# never lets `terraform init -upgrade` move it.
terraform {
  required_version = ">= 1.14.0"

  required_providers {
    neon = {
      source  = "kislerdm/neon"
      version = "0.18.0"
    }
  }
}
