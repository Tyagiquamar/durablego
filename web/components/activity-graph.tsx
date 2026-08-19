import type { ActivityView } from "../lib/dashboard-types"
import { StatusBadge } from "./status-badge"

export function ActivityGraph({ activities }: { activities: ActivityView[] }) {
  return <section className="activity-graph" aria-labelledby="activity-graph-title"><div className="section-heading"><div><p className="eyebrow">Dependency summary</p><h2 id="activity-graph-title">Activity graph</h2></div><span className="muted">Visual summary</span></div><div className="graph-nodes">{activities.map((activity) => <div className="graph-node" key={activity.id}><span className="mono">{activity.dependsOn.length ? `after ${activity.dependsOn.join(", ")}` : "entry activity"}</span><strong>{activity.name}</strong><StatusBadge status={activity.status} /></div>)}</div></section>
}