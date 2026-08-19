import type { EventView } from "../lib/dashboard-types"
import { formatDate } from "../lib/dashboard-types"

export function EventTimeline({ events }: { events: EventView[] }) {
  return <section className="event-timeline" aria-labelledby="event-timeline-title"><div className="section-heading"><div><p className="eyebrow">Durable history</p><h2 id="event-timeline-title">Execution events</h2></div></div>{events.length ? <ol>{events.map((event) => <li key={event.sequence}><span>{event.sequence}</span><div><strong>{event.type}</strong><p>{event.message}</p><time dateTime={event.at ?? undefined}>{formatDate(event.at)}</time></div></li>)}</ol> : <p className="muted">No event history was returned for this execution.</p>}</section>
}