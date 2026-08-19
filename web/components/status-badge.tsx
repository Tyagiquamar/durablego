import type { ActivityStatus, WorkflowStatus } from "../lib/dashboard-types"

export function StatusBadge({ status }: { status: WorkflowStatus | ActivityStatus }) {
  return <span className={`status-badge status-${status.replace("_", "-")}`}>{status.replace("_", " ")}</span>
}