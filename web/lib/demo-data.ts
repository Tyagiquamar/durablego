import type { DashboardData, FailureScene, WorkflowView } from "./dashboard-types"

const observedAt = "2026-08-19T14:30:00.000Z"

export const demoWorkflows: WorkflowView[] = [
  {
    id: "wf_order_fenced", namespace: "orders", definition: "checkout", status: "running", idempotencyKey: "checkout-1042", createdAt: "2026-08-19T14:08:00.000Z", updatedAt: "2026-08-19T14:29:00.000Z",
    activities: [
      { id: "act_validate", workflowId: "wf_order_fenced", name: "validate-order", taskQueue: "checkout", status: "completed", attempt: 1, maxAttempts: 3, leaseOwner: null, leaseExpiresAt: null, fencingToken: 1, nextAttemptAt: null, dependsOn: [], lastError: null, completedAt: "2026-08-19T14:10:00.000Z", retryBackoffMs: 1000 },
      { id: "act_charge", workflowId: "wf_order_fenced", name: "charge-card", taskQueue: "payments", status: "running", attempt: 2, maxAttempts: 3, leaseOwner: "worker-payments-b", leaseExpiresAt: "2026-08-19T14:31:00.000Z", fencingToken: 2, nextAttemptAt: null, dependsOn: ["validate-order"], lastError: "lease expired on worker-payments-a", completedAt: null, retryBackoffMs: 2000 },
      { id: "act_receipt", workflowId: "wf_order_fenced", name: "send-receipt", taskQueue: "notifications", status: "retry_pending", attempt: 0, maxAttempts: 3, leaseOwner: null, leaseExpiresAt: null, fencingToken: null, nextAttemptAt: null, dependsOn: ["charge-card"], lastError: null, completedAt: null, retryBackoffMs: 1000 },
    ],
    events: [
      { sequence: 1, workflowId: "wf_order_fenced", activityId: null, type: "workflow.started", message: "workflow created", at: "2026-08-19T14:08:00.000Z" },
      { sequence: 2, workflowId: "wf_order_fenced", activityId: "act_validate", type: "activity.completed", message: "validate-order completed", at: "2026-08-19T14:10:00.000Z" },
      { sequence: 3, workflowId: "wf_order_fenced", activityId: "act_charge", type: "activity.lease_expired", message: "lease expired and activity became ready", at: "2026-08-19T14:22:00.000Z" },
      { sequence: 4, workflowId: "wf_order_fenced", activityId: "act_charge", type: "activity.claimed", message: "charge-card claimed by worker-payments-b token 2", at: "2026-08-19T14:24:00.000Z" },
      { sequence: 5, workflowId: "wf_order_fenced", activityId: "act_charge", type: "activity.stale_completion_rejected", message: "completion rejected by fencing token 1 after token 2 claim", at: "2026-08-19T14:29:00.000Z" },
    ],
  },
  {
    id: "wf_export_retry", namespace: "reporting", definition: "daily-export", status: "running", idempotencyKey: null, createdAt: "2026-08-19T13:45:00.000Z", updatedAt: "2026-08-19T14:20:00.000Z",
    activities: [{ id: "act_export", workflowId: "wf_export_retry", name: "write-export", taskQueue: "exports", status: "retry_pending", attempt: 2, maxAttempts: 4, leaseOwner: null, leaseExpiresAt: null, fencingToken: 2, nextAttemptAt: "2026-08-19T14:32:00.000Z", dependsOn: [], lastError: "warehouse connection reset", completedAt: null, retryBackoffMs: 4000 }],
    events: [{ sequence: 1, workflowId: "wf_export_retry", activityId: null, type: "workflow.started", message: "workflow created", at: "2026-08-19T13:45:00.000Z" }, { sequence: 2, workflowId: "wf_export_retry", activityId: "act_export", type: "activity.retry_scheduled", message: "warehouse connection reset", at: "2026-08-19T14:20:00.000Z" }],
  },
  {
    id: "wf_invoice_done", namespace: "billing", definition: "invoice-close", status: "completed", idempotencyKey: "invoice-883", createdAt: "2026-08-19T12:02:00.000Z", updatedAt: "2026-08-19T12:08:00.000Z",
    activities: [{ id: "act_invoice", workflowId: "wf_invoice_done", name: "close-invoice", taskQueue: "billing", status: "completed", attempt: 1, maxAttempts: 3, leaseOwner: null, leaseExpiresAt: null, fencingToken: 1, nextAttemptAt: null, dependsOn: [], lastError: null, completedAt: "2026-08-19T12:08:00.000Z", retryBackoffMs: 1000 }],
    events: [{ sequence: 1, workflowId: "wf_invoice_done", activityId: null, type: "workflow.started", message: "workflow created", at: "2026-08-19T12:02:00.000Z" }, { sequence: 2, workflowId: "wf_invoice_done", activityId: null, type: "workflow.completed", message: "all activities completed", at: "2026-08-19T12:08:00.000Z" }],
  },
  {
    id: "wf_webhook_failed", namespace: "integrations", definition: "deliver-webhook", status: "failed", idempotencyKey: null, createdAt: "2026-08-19T11:30:00.000Z", updatedAt: "2026-08-19T11:42:00.000Z",
    activities: [{ id: "act_webhook", workflowId: "wf_webhook_failed", name: "post-webhook", taskQueue: "webhooks", status: "failed", attempt: 3, maxAttempts: 3, leaseOwner: null, leaseExpiresAt: null, fencingToken: 3, nextAttemptAt: null, dependsOn: [], lastError: "upstream returned 503 after final attempt", completedAt: null, retryBackoffMs: 3000 }],
    events: [{ sequence: 1, workflowId: "wf_webhook_failed", activityId: null, type: "workflow.started", message: "workflow created", at: "2026-08-19T11:30:00.000Z" }, { sequence: 2, workflowId: "wf_webhook_failed", activityId: "act_webhook", type: "activity.failed", message: "upstream returned 503 after final attempt", at: "2026-08-19T11:42:00.000Z" }, { sequence: 3, workflowId: "wf_webhook_failed", activityId: null, type: "workflow.failed", message: "activity post-webhook failed", at: "2026-08-19T11:42:00.000Z" }],
  },
]

export const failureScenes: FailureScene[] = [
  { id: "crash-recovery", title: "Crash recovery", failureWindow: "A worker stops after claiming work, before it can complete.", invariant: "Lease expiry returns unfinished work to the ready queue without discarding history.", eventSequence: ["activity.claimed", "activity.lease_expired", "activity.claimed"], workflowId: "wf_order_fenced" },
  { id: "stale-fencing", title: "Stale completion fencing", failureWindow: "A previous worker completes after the activity has been reclaimed.", invariant: "Token 1 cannot overwrite the token 2 claim.", eventSequence: ["activity.lease_expired", "activity.claimed token 2", "activity.stale_completion_rejected"], workflowId: "wf_order_fenced" },
  { id: "duplicate-start", title: "Duplicate workflow start", failureWindow: "A caller repeats a start request after losing its response.", invariant: "Namespace plus idempotency key resolves to one durable execution.", eventSequence: ["workflow.started", "idempotency key lookup", "existing workflow returned"], workflowId: "wf_invoice_done" },
  { id: "retry-exhaustion", title: "Retry exhaustion", failureWindow: "A retryable activity keeps failing until attempts are spent.", invariant: "The final attempt records the terminal workflow failure.", eventSequence: ["activity.retry_scheduled", "activity.failed", "workflow.failed"], workflowId: "wf_webhook_failed" },
]

export function demoDashboardData(): DashboardData {
  return { mode: "demo", state: "populated", workflows: demoWorkflows, observedAt, detailFailures: 0 }
}