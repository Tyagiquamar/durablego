import { EmptyState } from "../../components/empty-state"
import { DashboardUnavailable, PartialObservationNotice } from "../../components/dashboard-state"
import { WorkerTable } from "../../components/worker-table"
import { AutoRefresh } from "../../components/auto-refresh"
import { deriveWorkers, formatDate } from "../../lib/dashboard-types"
import { getDashboardData, resolveMode } from "../../lib/api"

export const dynamic = "force-dynamic"
export const maxDuration = 60

type PageProps = { searchParams: Promise<{ mode?: string | string[] }> }

export default async function WorkersPage({ searchParams }: PageProps) {
  const mode = resolveMode((await searchParams).mode)
  const data = await getDashboardData(mode)
  if (data.state === "unavailable") return <DashboardUnavailable mode={mode} title="Worker observation is unavailable." message={data.message} path="/workers" />
  const workers = deriveWorkers(data.workflows)
  return <><header className="page-heading"><div><p className="eyebrow">Derived worker view</p><h1>Current lease owners</h1></div><p>Observed {formatDate(data.observedAt)} from the newest {data.workflows.length} workflow details. This is not a worker registry.</p></header>{data.state === "partial" ? <PartialObservationNotice failedReads={data.detailFailures} /> : null}{mode === "live" ? <AutoRefresh intervalMs={15_000} /> : null}{workers.length ? <WorkerTable workers={workers} /> : <EmptyState title="No active lease owners" detail="The selected workflow window has no currently running activity leases." />}</>
}