import { EmptyState } from "../components/empty-state"
import { DashboardUnavailable, PartialObservationNotice } from "../components/dashboard-state"
import { MetricGrid } from "../components/metric-grid"
import { ProofTimeline } from "../components/proof-timeline"
import { WorkflowTable } from "../components/workflow-table"
import { AutoRefresh } from "../components/auto-refresh"
import { WhySection } from "../components/why-section"
import { getDashboardData, resolveMode } from "../lib/api"

export const dynamic = "force-dynamic"
export const maxDuration = 60

type PageProps = { searchParams: Promise<{ mode?: string | string[] }> }

export default async function Home({ searchParams }: PageProps) {
  const mode = resolveMode((await searchParams).mode)
  const data = await getDashboardData(mode)
  if (data.state === "unavailable") return <DashboardUnavailable mode={mode} title="Workflow evidence cannot be loaded." message={data.message} path="/" />
  return <><header className="page-heading"><div><p className="eyebrow">Durable workflow lab</p><h1>Execution proof, made inspectable.</h1></div><p>{mode === "demo" ? "A deterministic data set for reliability review." : `Observed ${data.workflows.length} recent workflows from the current API.`}</p></header>{data.state === "empty" ? <EmptyState title="No live executions" detail="The DurableGo API responded successfully, but it has no workflow executions." /> : <>{data.state === "partial" ? <PartialObservationNotice failedReads={data.detailFailures} /> : null}{mode === "live" ? <AutoRefresh /> : null}<MetricGrid workflows={data.workflows} /><section className="content-section"><div className="section-heading"><div><p className="eyebrow">Recent executions</p><h2>Workflow ledger</h2></div><a href={`/workflows?mode=${mode}`}>All executions</a></div><WorkflowTable workflows={data.workflows} mode={mode} compact /></section><ProofTimeline mode={mode} workflows={data.workflows} /><WhySection /></>}</>
}
