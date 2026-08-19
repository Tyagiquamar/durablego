import { CheckCircle2, Clock3, Layers3, PlayCircle, Timer } from "lucide-react"
import type { WorkflowView } from "../lib/dashboard-types"

export function MetricGrid({ workflows }: { workflows: WorkflowView[] }) {
  const activities = workflows.flatMap((workflow) => workflow.activities)
  const metrics = [
    { label: "Executions", value: workflows.length, icon: Layers3 },
    { label: "Running", value: workflows.filter((workflow) => workflow.status === "running").length, icon: PlayCircle },
    { label: "Completed", value: workflows.filter((workflow) => workflow.status === "completed").length, icon: CheckCircle2 },
    { label: "Active leases", value: activities.filter((activity) => activity.status === "running" && activity.leaseOwner).length, icon: Clock3 },
    { label: "Ready work", value: activities.filter((activity) => activity.status === "ready").length, icon: Timer },
  ]
  return <section className="metric-grid" aria-label="Execution summary">{metrics.map(({ label, value, icon: Icon }) => <div className="metric" key={label}><span><Icon aria-hidden="true" />{label}</span><strong>{value}</strong></div>)}</section>
}