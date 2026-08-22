const points = [
  {
    kicker: "The problem",
    title: "A workflow dies mid-payment. What happens to the charge?",
    body: `An agent submits an order pipeline — validate, pay, reserve inventory. The worker crashes between "payment sent" and "payment recorded". The client retries. Without durable state you either lose the order or send it twice, and no dashboard tells you which.`,
  },
  {
    kicker: "The guarantee",
    title: "Every step is a ledger entry before it is a side effect.",
    body: `Workflows and activities persist as event-sourced rows in PostgreSQL before dispatch. Claims go through SKIP LOCKED queues with monotonically increasing fencing tokens; duplicate starts collapse to one execution via idempotency keys, even under concurrent submission. The contract is at-least-once with fenced recovery — never silently exactly-once.`,
  },
  {
    kicker: "The proof",
    title: "The failure scenes are real kills, not diagrams.",
    body: `Integration tests SIGKILL a live worker subprocess mid-heartbeat, let the lease lapse, let a second worker reclaim, then replay the dead worker's completion — the API answers 409 with an activity.stale_completion_rejected audit event. The hosted demo feeds itself the same way: real pipelines, real retries, recoveries you can watch below.`,
  },
]

export function WhySection() {
  return (
    <section className="why">
      <p className="eyebrow">Why this exists</p>
      <h2 className="why-title">Durable workflows that survive their own workers</h2>
      <div className="why-grid">
        {points.map((p) => (
          <article key={p.kicker}>
            <p className="eyebrow">{p.kicker}</p>
            <h3>{p.title}</h3>
            <p>{p.body}</p>
          </article>
        ))}
      </div>
    </section>
  )
}
