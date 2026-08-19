"use client"

import { usePathname, useSearchParams } from "next/navigation"

export function modeHref(pathname: string, mode: "demo" | "live"): string {
  return `${pathname}?mode=${mode}`
}

export function ModeToggle() {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const activeMode = searchParams.get("mode") === "live" ? "live" : "demo"

  return (
    <div className="mode-toggle" role="group" aria-label="Dashboard data mode">
      <a aria-current={activeMode === "demo" ? "true" : undefined} className={activeMode === "demo" ? "active" : undefined} href={modeHref(pathname, "demo")}>Demo</a>
      <a aria-current={activeMode === "live" ? "true" : undefined} className={activeMode === "live" ? "active" : undefined} href={modeHref(pathname, "live")}>Live</a>
    </div>
  )
}