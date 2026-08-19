export type DashboardMode = "demo" | "live"

export type WorkflowStatus = "running" | "completed" | "failed" | "cancelled"
export type ActivityStatus = "ready" | "running" | "retry_pending" | "completed" | "failed" | "cancelled"

export type EventView = {
  sequence: number
  workflowId: string
  activityId: string | null
  type: string
  message: string
  at: string | null
}

export type ActivityView = {
  id: string
  workflowId: string
  name: string
  taskQueue: string
  status: ActivityStatus
  attempt: number
  maxAttempts: number
  leaseOwner: string | null
  leaseExpiresAt: string | null
  fencingToken: number | null
  nextAttemptAt: string | null
  dependsOn: string[]
  lastError: string | null
  completedAt: string | null
  retryBackoffMs: number | null
}

export type WorkflowView = {
  id: string
  namespace: string
  definition: string
  status: WorkflowStatus
  idempotencyKey: string | null
  createdAt: string | null
  updatedAt: string | null
  activities: ActivityView[]
  events: EventView[]
}

export type WorkerView = {
  id: string
  activeLeases: number
  workflows: string[]
  latestLeaseExpiry: string | null
}

export type DashboardData = {
  mode: DashboardMode
  state: "populated" | "empty" | "unavailable" | "partial"
  workflows: WorkflowView[]
  observedAt: string
  detailFailures: number
  message?: string
}

export type FailureScene = {
  id: "crash-recovery" | "stale-fencing" | "duplicate-start" | "retry-exhaustion"
  title: string
  failureWindow: string
  invariant: string
  eventSequence: string[]
  workflowId: string
}

export function deriveWorkers(workflows: WorkflowView[]): WorkerView[] {
  const workers = new Map<string, WorkerView>()
  for (const workflow of workflows) {
    for (const activity of workflow.activities) {
      if (!activity.leaseOwner || activity.status !== "running") continue
      const existing = workers.get(activity.leaseOwner) ?? {
        id: activity.leaseOwner,
        activeLeases: 0,
        workflows: [],
        latestLeaseExpiry: null,
      }
      existing.activeLeases += 1
      if (!existing.workflows.includes(workflow.id)) existing.workflows.push(workflow.id)
      if (!existing.latestLeaseExpiry || (activity.leaseExpiresAt && activity.leaseExpiresAt > existing.latestLeaseExpiry)) {
        existing.latestLeaseExpiry = activity.leaseExpiresAt
      }
      workers.set(existing.id, existing)
    }
  }
  return [...workers.values()].sort((left, right) => left.id.localeCompare(right.id))
}

export function formatDate(value: string | null): string {
  if (!value) return "not applicable"
  return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "medium", timeZone: "UTC" }).format(new Date(value)) + " UTC"
}

export function formatDuration(milliseconds: number | null): string {
  if (milliseconds === null) return "not applicable"
  if (milliseconds < 1000) return `${milliseconds} ms`
  return `${(milliseconds / 1000).toFixed(milliseconds % 1000 === 0 ? 0 : 1)} s`
}