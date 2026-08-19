import type { Metadata } from "next"
import { Suspense } from "react"
import { DashboardShell } from "../components/dashboard-shell"
import "./styles.css"

export const metadata: Metadata = {
  title: "DurableGo Dashboard",
  description: "Proof dashboard for durable workflow execution invariants.",
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body><Suspense fallback={children}><DashboardShell>{children}</DashboardShell></Suspense></body>
    </html>
  )
}

