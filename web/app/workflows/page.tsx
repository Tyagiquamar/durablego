import { EmptyState } from "../../components/empty-state"
import { DashboardUnavailable, PartialObservationNotice } from "../../components/dashboard-state"
import { WorkflowTable } from "../../components/workflow-table"
import { getDashboardData, resolveMode } from "../../lib/api"

type PageProps = { searchParams: Promise<{ mode?: string | string[] }> }

export default async function WorkflowsPage({ searchParams }: PageProps) {
  const mode = resolveMode((await searchParams).mode)
  const data = await getDashboardData(mode)
  if (data.state === "unavailable") return <DashboardUnavailable mode={mode} title="Execution data is not available." message={data.message} path="/workflows" />
  return <><header className="page-heading"><div><p className="eyebrow">Execution ledger</p><h1>Workflow executions</h1></div><p>{mode === "demo" ? "Deterministic records expose every proof state." : "Newest 20 API workflows, ordered by creation time."}</p></header>{data.state === "empty" ? <EmptyState title="No Live executions" detail="The DurableGo API responded successfully with an empty workflow list." /> : <>{data.state === "partial" ? <PartialObservationNotice failedReads={data.detailFailures} /> : null}<WorkflowTable workflows={data.workflows} mode={mode} /></>}</>
}