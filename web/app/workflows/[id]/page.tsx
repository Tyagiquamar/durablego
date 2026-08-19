import { notFound } from "next/navigation"
import { ActivityGraph } from "../../../components/activity-graph"
import { ActivityList } from "../../../components/activity-list"
import { DashboardUnavailable } from "../../../components/dashboard-state"
import { EventTimeline } from "../../../components/event-timeline"
import { StatusBadge } from "../../../components/status-badge"
import { formatDate } from "../../../lib/dashboard-types"
import { getWorkflowData, resolveMode } from "../../../lib/api"

type PageProps = { params: Promise<{ id: string }>; searchParams: Promise<{ mode?: string | string[] }> }

export default async function WorkflowDetailPage({ params, searchParams }: PageProps) {
  const { id } = await params
  const mode = resolveMode((await searchParams).mode)
  const { data, workflow } = await getWorkflowData(mode, id)
  if (!workflow && data.state === "empty") notFound()
  if (!workflow) return <DashboardUnavailable mode={mode} title="Execution detail is unavailable." message={data.message} path={`/workflows/${encodeURIComponent(id)}`} />
  return <><header className="page-heading"><div><p className="eyebrow">Workflow execution</p><h1 className="mono heading-id">{workflow.id}</h1></div><div className="workflow-detail-status"><StatusBadge status={workflow.status} /><p>{workflow.namespace} · {workflow.definition}<br />Created {formatDate(workflow.createdAt)}</p></div></header><ActivityGraph activities={workflow.activities} /><ActivityList activities={workflow.activities} /><EventTimeline events={workflow.events} /></>
}