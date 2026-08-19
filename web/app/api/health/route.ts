import { NextResponse } from "next/server"

export async function GET() {
  const baseURL = process.env.DURABLEGO_API_URL ?? "http://localhost:8080"

  try {
    const response = await fetch(`${baseURL}/healthz`, { cache: "no-store" })
    const body = await response.text()
    return new NextResponse(body, {
      headers: { "content-type": response.headers.get("content-type") ?? "application/json" },
      status: response.status,
    })
  } catch {
    return NextResponse.json({ status: "unavailable" }, { status: 503 })
  }
}