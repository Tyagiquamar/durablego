import type { DashboardMode, WorkflowView } from "../lib/dashboard-types"
import { formatDate } from "../lib/dashboard-types"

export function ProofTimeline({ mode, workflows }: { mode: DashboardMode; workflows: WorkflowView[] }) {
  const events = workflows.flatMap((workflow) => workflow.events.map((event) => ({ ...event, workflowId: workflow.id }))).filter((event) => event.type.includes("lease_expired") || event.type.includes("claimed") || event.type.includes("stale_completion_rejected")).slice(-4)
  const hasEvidence = events.some((event) => event.type.includes("stale_completion_rejected"))
  const title = mode === "live" && !hasEvidence ? "No matching live fencing evidence" : "A stale completion cannot win."
  const description = mode === "live" && !hasEvidence ? "The current observation does not contain a lease expiry, newer claim, and stale-completion rejection sequence." : "Ordered history remains durable when a lease expires and a newer claim takes ownership."
  return <section className="proof-timeline" aria-labelledby="fencing-proof"><div><p className="eyebrow">Fencing proof</p><h2 id="fencing-proof">{title}</h2><p>{description}</p></div>{events.length ? <ol>{events.map((event) => <li key={`${event.workflowId}-${event.sequence}`}><span>{event.sequence}</span><div><strong>{event.type}</strong><p>{event.message}</p><time dateTime={event.at ?? undefined}>{formatDate(event.at)}</time></div></li>)}</ol> : <p className="muted">No matching event sequence was returned.</p>}</section>
}