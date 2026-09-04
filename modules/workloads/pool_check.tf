# Whether a workload's compute_pool is usable, and the sentence shown when it is
# not. Both live in a module with no provider so that `terraform test` can plan
# them, which is the only reason this file exists rather than two inline
# expressions.
#
# This module reads eight remote data sources, so it cannot be planned in CI
# without AWS credentials, and `terraform validate` never evaluates a
# precondition's error_message. Between v4.2.0 and v4.4.1 that blind spot let a
# message which could not render ship four times, and it broke every deploy that
# had a Fargate service. ../compute_pool_check needs no provider and no network,
# so its own tests catch that before a release does.
#
# The two instances differ only in the words: a service and the backend open the
# sentence differently and are referred to differently at the end of it.

module "service_pool_check" {
  source     = "../compute_pool_check"
  pool_names = keys(local.pools)

  workloads = { for k, p in local.service_pools : k => {
    subject = "Service \"${k}\""
    noun    = "service"
    pool    = p
  } }
}

module "backend_pool_check" {
  source     = "../compute_pool_check"
  pool_names = keys(local.pools)

  workloads = {
    backend = {
      subject = "The backend"
      noun    = "backend"
      pool    = local.backend_pool
    }
  }
}
