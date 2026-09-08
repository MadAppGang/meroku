locals {
  # The collision ECS itself computes. RegisterTaskDefinition compares
  # secrets[].name against environment[].name within ONE container definition,
  # by exact string equality, and rejects the call if any name is in both:
  #
  #   ClientException: The secret name must be unique and not shared with any
  #   new or existing environment variables set on the container, such as
  #   'SCAN_CURSOR_STORE'.
  #
  # Exact equality is deliberate here too, and it is the reason var.workloads
  # insists on the RENDERED names. The upper-casing that turns an SSM parameter
  # ".../scan_cursor_store" into the secret "SCAN_CURSOR_STORE" has already
  # happened by the time the names arrive, so a lower-case parameter and an
  # upper-case YAML variable meet as one string and are caught. Applying any
  # further case folding HERE would go the other way and invent a collision ECS
  # does not have: a container may legitimately carry the environment variable
  # "scan_cursor_store" and the secret "SCAN_CURSOR_STORE" side by side, and
  # refusing to plan that would block a configuration AWS accepts.
  #
  # sort() only so the message is stable between runs; setintersection returns a
  # set, whose iteration order is Terraform's, not the caller's.
  collisions = {
    for k, w in var.workloads : k =>
    sort(tolist(setintersection(toset(w.environment), toset(w.secrets))))
  }

  valid = { for k, c in local.collisions : k => length(c) == 0 }

  # The names as they go INTO the sentence, and the reason this is a separate
  # local rather than a join() in the template below.
  #
  # Terraform renders a precondition's error_message BEFORE it tests the
  # condition, and for EVERY instance of the resource — so this string is built
  # for the clean workloads too, where the list is empty. A bare join() would
  # render "The backend would set  both as ...", which is not a sentence and,
  # worse, is a sentence nobody would ever see failing: the interesting text
  # would only ever be exercised by the failing case. Keeping it total means
  # every plan of every workload renders the same string a broken one shows.
  #
  # This is the same class of defect as the compute_pool null (v4.2.0, shipped
  # through four releases, broke every Fargate deploy). See
  # ../compute_pool_check.
  rendered_names = {
    for k, c in local.collisions : k =>
    length(c) > 0 ? join(", ", c) : "(none)"
  }

  message = { for k, w in var.workloads : k => <<-EOT
    ${w.subject} would set ${local.rendered_names[k]} both as a plain environment variable and as a secret. ECS refuses to register a task definition where one name appears in both, so this does not fail here for tidiness — it fails mid-apply, at RegisterTaskDefinition, with "ClientException: The secret name must be unique and not shared with any new or existing environment variables set on the container".

    Nobody need have touched that name for this to start failing. Two independent mechanisms write into one container's environment and neither knows about the other. The environment side is DECLARED: ${w.yaml_field} in ${w.yaml_file}, plus the variables meroku always sets itself. The secret side is DISCOVERED: every SSM parameter under ${w.ssm_path} becomes a secret named after its last path segment, UPPER-CASED — so a parameter whose last segment is written in lower case collides just the same, and whoever created it broke the next apply anybody runs, on a variable they never edited.

    Keep exactly one of the two. Either

      - delete the SSM parameter under ${w.ssm_path}
        whose last segment upper-cases to that name, if the value in
        ${w.yaml_file} is the one you want; or

      - remove the name from ${w.yaml_field} in ${w.yaml_file}
        and re-render, if the SSM value is the one you want. A secret is
        the better home for anything sensitive anyway, and it can be
        changed without a terraform apply.

    If the name is one meroku sets itself — AWS_REGION and the rest of the list in ${w.defaults_source} — only the first option is open, because it is not in ${w.yaml_file} to remove.
  EOT
  }
}
