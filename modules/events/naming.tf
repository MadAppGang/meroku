# Every capped AWS name this module builds, through the shared cascade. See
# modules/naming. All of them are new resources, so there is no legacy form.
#
# Queues cap at 80 and rules at 64. A rule carries two user-supplied fields, the
# consumer and the subscription key, so it is the one most likely to reach form 3.
# app/events.go computes the queue names with the same cascade (awsName) to
# refuse an extensions.sqs_queues entry of the same name.

module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = merge(
    { for c in keys(local.consumers) : "queue/${c}" => {
      parts = ["events", c]
      limit = 80
    } },
    { for c in keys(local.consumers) : "dlq/${c}" => {
      parts = ["events", c, "dlq"]
      limit = 80
    } },
    { for c in keys(local.consumers) : "delivery_dlq/${c}" => {
      parts = ["events", c, "delivery-dlq"]
      limit = 80
    } },
    { for k, s in local.subscriptions : "rule/${k}" => {
      parts = ["events", s.consumer, s.key]
      limit = 64
    } },
  )
}
