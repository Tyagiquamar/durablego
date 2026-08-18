export interface Workflow {
  ID: string
  Namespace: string
  Name: string
  Status: "running" | "completed" | "failed" | "cancelled"
}

export async function getWorkflows(): Promise<Workflow[]> {
  const baseURL = process.env.NEXT_PUBLIC_DURABLEGO_API ?? "http://localhost:8080"
  try {
    const response = await fetch(`${baseURL}/v1/workflows`, { cache: "no-store" })
    if (!response.ok) {
      return []
    }
    const body = (await response.json()) as { workflows?: Workflow[] }
    return body.workflows ?? []
  } catch {
    return []
  }
}

