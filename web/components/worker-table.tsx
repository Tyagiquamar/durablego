import type { WorkerView } from "../lib/dashboard-types"
import { formatDate } from "../lib/dashboard-types"

export function WorkerTable({ workers }: { workers: WorkerView[] }) {
  return <section className="worker-table" aria-label="Derived lease owners"><div className="worker-head"><span>Lease owner</span><span>Active leases</span><span>Workflow executions</span><span>Latest expiry</span></div>{workers.map((worker) => <article className="worker-row" key={worker.id}><strong data-label="Lease owner">{worker.id}</strong><span data-label="Active leases">{worker.activeLeases}</span><span data-label="Workflow executions" className="mono">{worker.workflows.join(", ")}</span><time data-label="Latest expiry" className="mono" dateTime={worker.latestLeaseExpiry ?? undefined}>{formatDate(worker.latestLeaseExpiry)}</time></article>)}</section>
}