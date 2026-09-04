locals {
  # The pool name as it goes INTO the sentence. A workload on Fargate has no
  # pool, and a null in a string template is what broke every Fargate plan
  # between v4.2.0 and v4.4.1: Terraform renders a precondition's error_message
  # BEFORE it tests the condition, and for every instance of the resource, so a
  # message that cannot render kills a plan that had nothing wrong with it.
  # Rendering the null as "" is what makes the message total.
  rendered_pool = { for k, w in var.workloads : k => w.pool == null ? "" : w.pool }

  # Null means Fargate, which needs no pool. Anything else must name a pool that
  # is present AND enabled; var.pool_names carries only the enabled ones.
  #
  # A conditional, not `w.pool == null || contains(...)`. Terraform evaluates
  # only the branch a conditional takes, but `||` evaluates both operands on the
  # version CI pins (1.9.8), where `contains(list, null)` is "Invalid value for
  # \"value\" parameter: argument must not be null". Terraform 1.16 short-circuits
  # and hides it, which is how the || form passed locally and failed in CI. The
  # preconditions this module replaces carried the same shape.
  valid = { for k, w in var.workloads : k => w.pool == null ? true : contains(var.pool_names, w.pool) }

  # The half of the sentence that tells the reader what to do about it. With no
  # pools defined at all, the old text offered an empty list and ended on a bare
  # period ("point the service at one of these: ."), which is the most likely
  # state for someone hitting this: they set runtime "ec2" and never wrote a
  # compute block.
  remedy = { for k, w in var.workloads : k => length(var.pool_names) > 0
    ? "Add a pool with that name under compute.pools, set enabled: true on it if it is disabled, or point the ${w.noun} at one of these: ${join(", ", var.pool_names)}."
    : "This project defines no compute pools, so a ${w.noun} can only use runtime \"fargate\" until one is added under compute.pools."
  }

  # Rendered for EVERY workload, the valid ones included, because that is what
  # Terraform does with an error_message and therefore what the test has to
  # exercise. A message that only renders for the failing cases is the bug.
  message = { for k, w in var.workloads : k =>
    "${w.subject} sets runtime \"ec2\" with compute pool \"${local.rendered_pool[k]}\", which is not an enabled compute pool. ${local.remedy[k]}"
  }
}
