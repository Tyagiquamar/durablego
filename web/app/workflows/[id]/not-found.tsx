"use client"

import { useSearchParams } from "next/navigation"

export default function NotFound() {
	const mode = useSearchParams().get("mode") === "live" ? "live" : "demo"
	return <section className="route-state"><p className="eyebrow">Execution not found</p><h1>This workflow is not in the selected source.</h1><p>Return to the execution ledger to inspect the workflows that are currently available.</p><a className="reload-link" href={`/workflows?mode=${mode}`}>View executions</a></section>
}