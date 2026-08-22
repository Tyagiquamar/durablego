import type { DashboardMode } from "../lib/dashboard-types"

export function DashboardLoading({ label }: { label: string }) {
  return <section className="route-state" aria-live="polite"><span className="eyebrow">{label}</span><div className="loading-lines" aria-hidden="true"><i /><i /><i /></div></section>
}

export function DashboardUnavailable({ mode, title, message, path }: { mode: DashboardMode; title: string; message?: string; path: string }) {
  return <section className="route-state" role="alert"><p className="eyebrow">{mode} source unavailable</p><h1>{title}</h1><p>{message}</p><p className="cold-start-note">The hosted engine runs on a free tier and sleeps when idle — the first request can take up to a minute. This page retries for ~45s before giving up; reloading usually finds it awake.</p><a className="reload-link" href={mode === "demo" ? `${path}?mode=demo` : path}>Reload this source</a></section>
}

export function PartialObservationNotice({ failedReads }: { failedReads: number }) {
  return <p className="observation-note" role="status">Incomplete observation: {failedReads} workflow detail read{failedReads === 1 ? "" : "s"} failed; successful records remain visible.</p>
}