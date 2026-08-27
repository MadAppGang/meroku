# The one place meroku decides what an AWS resource is called.
#
# Every AWS name has a hard cap and the caps are not generous: 32 characters for
# a load balancer or target group, 40 for App Runner, 63 for an S3 bucket or RDS
# identifier, 64 for an IAM role, a Lambda, an EventBridge rule or a scheduler,
# 80 for an SQS queue. A template like "${project}-service-${name}-tg-${env}"
# spends 13 of a 32-character budget on the words "service" and "tg", which
# leaves seven characters for the service name — and then the AWS provider
# rejects the plan with `"name" cannot be longer than 32 characters`, naming a
# line of Terraform rather than the service that is actually too long.
#
# Terraform has no user-defined functions (verified against 1.15.8: a top-level
# `functions` block is "not expected here"), so the only way to write this rule
# once is a module. Callers hand over a map of requests and get back a map of
# names.
#
# THE CASCADE — three forms, first that fits wins.
#
#   1. `legacy`, verbatim.
#      Present only to protect what already exists. Most AWS names are ForceNew,
#      and some cannot even be deleted while something references them: a target
#      group attached to a listener rule, for one. Renaming those does not
#      "update" anything, it destroys and recreates, and the destroy blocks. So
#      any name that fits today is returned byte-for-byte and no plan moves.
#
#   2. `project + parts + env`, joined by the separator.
#      The decoration is gone and nothing else is. Words like "service", "tg"
#      and "task" identify nothing — the caller already knows which resource it
#      is asking about, and the tags carry Project and Environment — so dropping
#      them costs no information and buys back ten to fifteen characters. This
#      is where a project of ordinary size lands.
#
#   3. `parts + digest`, with project and env collapsed into eight hex
#      characters.
#      Reached only when project and env cannot fit beside the identity at all.
#      The identity is what a human scans for in a console listing, so it is the
#      last thing given up, never the first.
#
# WHY THE DIGEST IS UNCONDITIONAL IN FORM 3
#
# It hashes project, env and the WHOLE parts list, so it stays distinct when the
# readable head is truncated to a shared prefix, and when two environments share
# one AWS account. Terraform builds a `for` map in a single pass with no
# cross-entry visibility, so there is no way to add a digest only on collision;
# adding it always is what makes uniqueness independent of what survives.
#
# WHAT THIS MODULE DOES NOT DO
#
# It does not validate the character set. AWS disagrees with itself about
# underscores and about leading digits, per resource type, and a caller that
# passes a legal `legacy` and legal `parts` cannot produce an illegal name here.
# The digest is lowercase hex and the separator is one of two legal characters.

locals {
  # Form 2. project and env return to the ends; the caller's parts hold the
  # middle in the order it gave them.
  full = { for k, r in var.requests :
    k => "${join(r.separator, concat([var.project], r.parts, [var.env]))}${r.suffix}"
  }

  # "|" cannot appear in any AWS name, so it is an unambiguous field separator
  # here: ["a-b"] and ["a", "b"] hash differently, as they must.
  digest = { for k, r in var.requests :
    k => substr(md5(join("|", concat([var.project, var.env], r.parts))), 0, 8)
  }

  # Budget for the readable head of form 3: the limit, less the separator and
  # the eight-character digest, less any suffix that has to survive.
  head_budget = { for k, r in var.requests : k => r.limit - 9 - length(r.suffix) }

  # substr clamps rather than erroring when the string is shorter than the
  # requested length (verified: substr("short", 0, 23) == "short"), so a short
  # identity passes through whole and only a genuinely long one is cut.
  short = { for k, r in var.requests :
    k => format("%s%s%s%s",
      substr(join(r.separator, r.parts), 0, local.head_budget[k]),
      r.separator,
      local.digest[k],
      r.suffix,
    )
  }

  # compact() drops legacy when the caller left it empty, so a brand-new
  # resource starts the cascade at form 2 and never inherits decoration it was
  # not already carrying.
  candidates = { for k, r in var.requests : k => compact([
    r.legacy,
    local.full[k],
    local.short[k],
  ]) }

  # Form 3 is <= limit by construction (head_budget + separator + 8 + suffix),
  # so this index always finds something.
  names = { for k, r in var.requests :
    k => [for n in local.candidates[k] : n if length(n) <= r.limit][0]
  }

  # Which form each name landed on. Exposed because "did my name just change
  # shape?" is the question a reviewer asks about this module, and counting
  # characters by hand to answer it is how mistakes get in.
  forms = { for k, r in var.requests :
    k => (r.legacy == "" ? 1 : 0) + index([for n in local.candidates[k] : length(n) <= r.limit], true) + 1
  }
}
