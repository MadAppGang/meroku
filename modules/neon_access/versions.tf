# cyrilgdn/postgresql is pinned to one exact release: the role resource's
# search_path type (list(string)) and its rename on a name change are taken
# from that release's source.
terraform {
  required_version = ">= 1.14.0"

  required_providers {
    postgresql = {
      source  = "cyrilgdn/postgresql"
      version = "1.27.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}
