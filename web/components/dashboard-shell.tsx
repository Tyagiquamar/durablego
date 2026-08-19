"use client"

import { Activity, AlertTriangle, Cable, LayoutDashboard, Users } from "lucide-react"
import { usePathname, useSearchParams } from "next/navigation"
import type { ReactNode } from "react"
import { modeHref, ModeToggle } from "./mode-toggle"

const navigation = [
  { href: "/", label: "Overview", icon: LayoutDashboard },
  { href: "/workflows", label: "Executions", icon: Activity },
  { href: "/workers", label: "Workers", icon: Users },
  { href: "/failure-scenes", label: "Failure scenes", icon: AlertTriangle },
]

export function DashboardShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const mode = searchParams.get("mode") === "live" ? "live" : "demo"

  return (
    <div className="dashboard-frame">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <aside className="dashboard-sidebar">
        <a className="brand" href={modeHref("/", mode)}><Cable aria-hidden="true" /><span>DurableGo</span><small>Execution console</small></a>
        <nav className="primary-nav" aria-label="Dashboard navigation">
          {navigation.map(({ href, label, icon: Icon }) => {
            const active = href === "/" ? pathname === "/" : pathname.startsWith(href)
            return <a aria-current={active ? "page" : undefined} className={active ? "active" : ""} href={modeHref(href, mode)} key={href}><Icon aria-hidden="true" /><span>{label}</span></a>
          })}
        </nav>
        <div className="sidebar-footnote"><span>READ ONLY</span><p>Durable state and lease evidence.</p></div>
      </aside>
      <div className="dashboard-workspace">
        <header className="runtime-bar"><div className="source-label" aria-live="polite"><span className={mode === "demo" ? "source-dot demo" : "source-dot live"} aria-hidden="true" /><span>{mode === "demo" ? "Demo evidence" : "Live API observation"}</span></div><ModeToggle /></header>
        <main id="main-content" className="dashboard-main">{children}</main>
      </div>
    </div>
  )
}