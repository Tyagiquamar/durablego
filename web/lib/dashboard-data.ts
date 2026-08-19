import { demoDashboardData, demoWorkflows } from "./demo-data"
import type { ActivityStatus, ActivityView, DashboardData, DashboardMode, EventView, WorkflowStatus, WorkflowView } from "./dashboard-types"

type APIRecord = Record<string, unknown>

const statuses = new Set<WorkflowStatus>(["running", "completed", "failed", "cancelled"])
const activityStatuses = new Set<ActivityStatus>(["ready", "running", "retry_pending", "completed", "failed", "cancelled"])

export function resolveMode(mode?: string | string[]): DashboardMode {
  return (Array.isArray(mode) ? mode[0] : mode) === "live" ? "live" : "demo"
}

export async function getDashboardData(mode: DashboardMode, includeDetails = true): Promise<DashboardData> {
  if (mode === "demo") return demoDashboardData()

  try {
    const response = await fetchAPI("/v1/workflows")
    if (!response.ok) return unavailable(mode, `The workflow list returned HTTP ${response.status}.`)
    const body = (await response.json()) as APIRecord
    const workflowRecords = requiredArray(body.workflows ?? body.Workflows, "workflows")
    const workflows = workflowRecords.map(asRecord).map(toWorkflow).sort(newestFirst).slice(0, 20)
    if (workflows.length === 0) return { mode, state: "empty", workflows, observedAt: new Date().toISOString(), detailFailures: 0 }
    if (!includeDetails) return { mode, state: "populated", workflows, observedAt: new Date().toISOString(), detailFailures: 0 }
    const details = await Promise.allSettled(workflows.map((workflow) => getLiveWorkflow(workflow.id)))
    const hydrated = details.flatMap((detail, index) => detail.status === "fulfilled" ? [detail.value] : [workflows[index]])
    const detailFailures = details.filter((detail) => detail.status === "rejected").length
    return { mode, state: detailFailures ? "partial" : "populated", workflows: hydrated, observedAt: new Date().toISOString(), detailFailures }
  } catch (error) {
    return unavailable(mode, error instanceof Error && error.message.startsWith("malformed") ? "The workflow list response was malformed." : "The DurableGo API could not be reached.")
  }
}

export async function getWorkflowData(mode: DashboardMode, id: string): Promise<{ data: DashboardData; workflow?: WorkflowView }> {
  if (mode === "demo") {
    const workflow = demoWorkflows.find((record) => record.id === id)
    return { data: demoDashboardData(), workflow }
  }
  try {
    const workflow = await getLiveWorkflow(id)
    return { data: { mode, state: "populated", workflows: [workflow], observedAt: new Date().toISOString(), detailFailures: 0 }, workflow }
  } catch (error) {
    const message = error instanceof Error && error.message === "not-found" ? undefined : "This workflow could not be loaded from the DurableGo API."
    return { data: message ? unavailable(mode, message) : { mode, state: "empty", workflows: [], observedAt: new Date().toISOString(), detailFailures: 0 } }
  }
}

async function getLiveWorkflow(id: string): Promise<WorkflowView> {
  const response = await fetchAPI(`/v1/workflows/${encodeURIComponent(id)}`)
  if (response.status === 404) throw new Error("not-found")
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  const body = (await response.json()) as APIRecord
  const workflow = toWorkflow(requiredRecord(body.workflow ?? body.Workflow, "workflow"))
  workflow.activities = requiredArray(body.activities ?? body.Activities, "activities").map(asRecord).map(toActivity)
  workflow.events = requiredArray(body.events ?? body.Events, "events").map(asRecord).map(toEvent).sort((left, right) => left.sequence - right.sequence)
  return workflow
}

function fetchAPI(path: string): Promise<Response> {
  return fetch(`${apiBaseUrl()}${path}`, { cache: "no-store", signal: AbortSignal.timeout(5_000) })
}

function apiBaseUrl(): string {
  return process.env.DURABLEGO_API_URL ?? "http://localhost:8080"
}

function unavailable(mode: DashboardMode, message: string): DashboardData {
  return { mode, state: "unavailable", workflows: [], observedAt: new Date().toISOString(), detailFailures: 0, message }
}

function toWorkflow(value: APIRecord): WorkflowView {
  return {
    id: requiredString(value, "ID", "id"), namespace: requiredString(value, "Namespace", "namespace"), definition: requiredString(value, "Name", "name", "Definition", "definition"),
    status: workflowStatus(value), idempotencyKey: nullableString(value, "IdempotencyKey", "idempotency_key", "idempotencyKey"),
    createdAt: timestamp(value, "CreatedAt", "created_at", "createdAt"), updatedAt: timestamp(value, "UpdatedAt", "updated_at", "updatedAt"), activities: [], events: [],
  }
}

function toActivity(value: APIRecord): ActivityView {
  const retryBackoff = number(value, "RetryBackoff", "retry_backoff", "retryBackoff")
  return {
    id: string(value, "ID", "id"), workflowId: string(value, "WorkflowID", "workflow_id", "workflowId"), name: string(value, "Name", "name"), taskQueue: string(value, "TaskQueue", "task_queue", "taskQueue"),
    status: activityStatus(value), attempt: number(value, "Attempt", "attempt"), maxAttempts: number(value, "MaxAttempts", "max_attempts", "maxAttempts"),
    leaseOwner: nullableString(value, "LeaseOwner", "lease_owner", "leaseOwner"), leaseExpiresAt: timestamp(value, "LeaseExpiresAt", "lease_expires_at", "leaseExpiresAt"),
    fencingToken: nullableNumber(value, "FencingToken", "fencing_token", "fencingToken"), nextAttemptAt: timestamp(value, "NextAttemptAt", "next_attempt_at", "nextAttemptAt"),
    dependsOn: asArray(value.DependsOn ?? value.depends_on ?? value.dependsOn).map(String), lastError: nullableString(value, "LastError", "last_error", "lastError"),
    completedAt: timestamp(value, "CompletedAt", "completed_at", "completedAt"), retryBackoffMs: retryBackoff ? Math.round(retryBackoff / 1_000_000) : null,
  }
}

function toEvent(value: APIRecord): EventView {
  return { sequence: number(value, "Sequence", "sequence"), workflowId: string(value, "WorkflowID", "workflow_id", "workflowId"), activityId: nullableString(value, "ActivityID", "activity_id", "activityId"), type: string(value, "Type", "type"), message: string(value, "Message", "message"), at: timestamp(value, "At", "at") }
}

function newestFirst(left: WorkflowView, right: WorkflowView): number {
  const time = (right.createdAt ?? "").localeCompare(left.createdAt ?? "")
  return time || right.id.localeCompare(left.id)
}

function asRecord(value: unknown): APIRecord { return value && typeof value === "object" ? value as APIRecord : {} }
function requiredRecord(value: unknown, field: string): APIRecord { if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(`malformed ${field}`); return value as APIRecord }
function requiredArray(value: unknown, field: string): unknown[] { if (!Array.isArray(value)) throw new Error(`malformed ${field}`); return value }
function asArray(value: unknown): unknown[] { return Array.isArray(value) ? value : [] }
function valueOf(record: APIRecord, ...keys: string[]): unknown { return keys.map((key) => record[key]).find((value) => value !== undefined) }
function string(record: APIRecord, ...keys: string[]): string { const value = valueOf(record, ...keys); return typeof value === "string" ? value : "" }
function requiredString(record: APIRecord, ...keys: string[]): string { const value = string(record, ...keys); if (!value) throw new Error(`malformed ${keys[0]}`); return value }
function nullableString(record: APIRecord, ...keys: string[]): string | null { const value = string(record, ...keys); return value || null }
function number(record: APIRecord, ...keys: string[]): number { const value = valueOf(record, ...keys); return typeof value === "number" ? value : 0 }
function nullableNumber(record: APIRecord, ...keys: string[]): number | null { const value = valueOf(record, ...keys); return typeof value === "number" && value !== 0 ? value : null }
function timestamp(record: APIRecord, ...keys: string[]): string | null { const value = string(record, ...keys); if (!value || value.startsWith("0001-01-01")) return null; if (Number.isNaN(Date.parse(value))) throw new Error(`malformed ${keys[0]}`); return value }
function workflowStatus(record: APIRecord): WorkflowStatus { const value = string(record, "Status", "status"); if (!statuses.has(value as WorkflowStatus)) throw new Error("malformed workflow status"); return value as WorkflowStatus }
function activityStatus(record: APIRecord): ActivityStatus { const value = string(record, "Status", "status"); if (!activityStatuses.has(value as ActivityStatus)) throw new Error("malformed activity status"); return value as ActivityStatus }