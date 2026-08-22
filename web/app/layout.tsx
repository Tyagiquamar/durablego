import type { Metadata } from "next"
import { Geist, Geist_Mono, Libre_Baskerville } from "next/font/google"
import type { ReactNode } from "react"
import { Suspense } from "react"
import { DashboardShell } from "../components/dashboard-shell"
import "./styles.css"

const geistSans = Geist({
  subsets: ["latin"],
  variable: "--font-sans",
})

const geistMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
})

const libreBaskerville = Libre_Baskerville({
  subsets: ["latin"],
  weight: ["400", "700"],
  variable: "--font-display",
})

export const metadata: Metadata = {
  title: "DurableGo — Execution Proof Console",
  description:
    "A durable workflow engine that proves its own guarantees: SIGKILL-tested recovery, fencing tokens, and an inspectable event ledger.",
}

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" className={`${geistSans.variable} ${geistMono.variable} ${libreBaskerville.variable}`}>
      <body>
        <Suspense fallback={children}>
          <DashboardShell>{children}</DashboardShell>
        </Suspense>
      </body>
    </html>
  )
}
