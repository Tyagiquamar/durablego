import type { Metadata } from "next"
import "./styles.css"

export const metadata: Metadata = {
  title: "DurableGo Dashboard",
  description: "Proof dashboard for durable workflow execution invariants.",
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  )
}

