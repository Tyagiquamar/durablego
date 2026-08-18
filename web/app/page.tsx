import { Activity, Database, GitBranch, ShieldCheck } from "lucide-react"
import { getWorkflows } from "../lib/api"

const scenes = [
  {
    title: "Crash Recovery",
    invariant: "Expired leases become claimable without losing event history.",
    status: "Ready for scripted demo",
  },
  {
    title: "Stale Fencing",
    invariant: "Old fencing tokens cannot complete reclaimed activity attempts.",
    status: "Covered by failure test",
  },
  {
    title: "Duplicate Start",
    invariant: "Namespace plus idempotency key maps repeated starts to one execution.",
    status: "Covered by integration test",
  },
  {
    title: "Retry Exhaustion",
    invariant: "Bounded retries end in a visible terminal failure.",
    status: "Covered by unit test",
  },
]

export default async function Home() {
  const workflows = await getWorkflows()
  const stats = [
    ["Running", String(workflows.filter((workflow) => workflow.Status === "running").length)],
    ["Completed", String(workflows.filter((workflow) => workflow.Status === "completed").length)],
    ["Failed", String(workflows.filter((workflow) => workflow.Status === "failed").length)],
    ["Workers Online", "2"],
  ]

  return (
    <main>
      <header className="topbar">
        <div>
          <p className="eyebrow">Durable workflow lab</p>
          <h1>DurableGo</h1>
        </div>
        <a href="http://localhost:8080/healthz">API Health</a>
      </header>

      <section className="summary">
        <div>
          <ShieldCheck aria-hidden />
          <p>At-least-once activity execution with explicit negative guarantees for external side effects.</p>
        </div>
        <div>
          <Database aria-hidden />
          <p>PostgreSQL schema owns workflow state, activity leases, idempotency, and ordered history.</p>
        </div>
        <div>
          <GitBranch aria-hidden />
          <p>DAG-ready execution model unlocks downstream work only after dependencies complete.</p>
        </div>
        <div>
          <Activity aria-hidden />
          <p>Prometheus-style metrics and event timelines make failure proofs inspectable.</p>
        </div>
      </section>

      <section className="metrics" aria-label="Fleet summary">
        {stats.map(([label, value]) => (
          <div key={label}>
            <span>{label}</span>
            <strong>{value}</strong>
          </div>
        ))}
      </section>

      <section className="scenes">
        <div className="section-title">
          <p className="eyebrow">Proof scenes</p>
          <h2>What fails, what holds, where to look.</h2>
        </div>
        <div className="scene-list">
          {scenes.map((scene, index) => (
            <article key={scene.title}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div>
                <h3>{scene.title}</h3>
                <p>{scene.invariant}</p>
              </div>
              <strong>{scene.status}</strong>
            </article>
          ))}
        </div>
      </section>
    </main>
  )
}
