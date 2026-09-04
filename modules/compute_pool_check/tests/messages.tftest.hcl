# The contract, in the order the contract matters.
#
#   1. The message renders for EVERY workload, Fargate included.
#      (fargate_workload_still_renders_a_message)
#   2. A valid pool passes, a missing or disabled one does not.
#      (a_pool_that_exists_is_valid / a_missing_pool_is_invalid)
#   3. The sentence never trails off into a bare period.
#      (no_pools_defined_says_so / no_message_ends_in_a_bare_period)
#
# Rule 1 is why this module exists. modules/workloads reads eight remote data
# sources and cannot be planned in CI, and `terraform validate` never evaluates
# a precondition's error_message, so between v4.2.0 and v4.4.1 a message that
# interpolated a null shipped four times and broke every Fargate deploy. This
# module has no provider, so `terraform test` plans it with no credentials and
# no network, and a message that cannot render fails here instead of there.
#
# Run: terraform test  (from modules/compute_pool_check)

variables {
  pool_names = ["general", "spot"]

  workloads = {
    # Fargate. The pool is null, which is the case that used to crash.
    web = {
      subject = "Service \"web\""
      noun    = "service"
      pool    = null
    }
    # On a pool that exists.
    worker = {
      subject = "Service \"worker\""
      noun    = "service"
      pool    = "general"
    }
    # On a pool that does not.
    api = {
      subject = "Service \"api\""
      noun    = "service"
      pool    = "gone"
    }
  }
}

# THE regression test. It asserts almost nothing about the text, because the
# assertion is not the point: rendering the map at all is. A raw
# "${w.pool}" here fails the run before any assert is reached, with
# "Invalid template interpolation value ... The expression result is null."
run "fargate_workload_still_renders_a_message" {
  command = apply

  assert {
    condition     = length(output.message["web"]) > 0
    error_message = "A Fargate workload must still render a message. This is the v4.2.0 regression: Terraform renders error_message before it tests the condition, so a message that needs a non-null pool kills a plan that is entirely correct."
  }

  assert {
    condition     = output.valid["web"]
    error_message = "A workload with no pool runs on Fargate and must be valid, got invalid."
  }
}

run "a_pool_that_exists_is_valid" {
  command = apply

  assert {
    condition     = output.valid["worker"]
    error_message = "Pool \"general\" is in pool_names, so the workload must be valid."
  }
}

run "a_missing_pool_is_invalid" {
  command = apply

  assert {
    condition     = output.valid["api"] == false
    error_message = "Pool \"gone\" is not in pool_names, so the workload must be invalid."
  }

  # The whole sentence, byte for byte. This is the text a user reads at the
  # moment their deploy stops, and it is the only thing telling them what to do.
  assert {
    condition     = output.message["api"] == "Service \"api\" sets runtime \"ec2\" with compute pool \"gone\", which is not an enabled compute pool. Add a pool with that name under compute.pools, set enabled: true on it if it is disabled, or point the service at one of these: general, spot."
    error_message = "Message changed: ${output.message["api"]}"
  }
}

# With no pools at all the old text offered an empty list and ended on a bare
# period: "point the service at one of these: ." That is the most likely state
# for anyone hitting this error, because they set runtime "ec2" and never wrote
# a compute block.
run "no_pools_defined_says_so" {
  command = apply

  variables {
    pool_names = []
    workloads = {
      backend = {
        subject = "The backend"
        noun    = "backend"
        pool    = "gone"
      }
    }
  }

  assert {
    condition     = output.message["backend"] == "The backend sets runtime \"ec2\" with compute pool \"gone\", which is not an enabled compute pool. This project defines no compute pools, so a backend can only use runtime \"fargate\" until one is added under compute.pools."
    error_message = "Message changed: ${output.message["backend"]}"
  }
}

# The general form of the same rule, so a future edit to the remedy text cannot
# reintroduce a trailing empty list in some case nobody wrote a run for.
run "no_message_ends_in_a_bare_period" {
  command = apply

  variables {
    pool_names = []
    workloads = {
      fargate = { subject = "Service \"fargate\"", noun = "service", pool = null }
      ec2     = { subject = "Service \"ec2\"", noun = "service", pool = "gone" }
    }
  }

  assert {
    condition     = alltrue([for k, m in output.message : !endswith(m, ": .")])
    error_message = "A message trails off into an empty list: ${jsonencode({ for k, m in output.message : k => m if endswith(m, ": .") })}"
  }
}
