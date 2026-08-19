import { DatabaseZap } from "lucide-react"

export function EmptyState({ title, detail }: { title: string; detail: string }) {
  return <section className="empty-state" aria-live="polite"><DatabaseZap aria-hidden="true" /><h2>{title}</h2><p>{detail}</p></section>
}