"use client"

export default function Error({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return <section className="route-state" role="alert"><span className="eyebrow">Dashboard unavailable</span><h1>That request could not be completed.</h1><p>The selected mode and navigation remain available. Reload to try the same URL again.</p><button onClick={reset} type="button">Reload data</button></section>
}