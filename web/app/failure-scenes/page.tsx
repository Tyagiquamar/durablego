import { FailureSceneCard } from "../../components/failure-scene-card"
import { failureScenes } from "../../lib/demo-data"
import { resolveMode } from "../../lib/api"

type PageProps = { searchParams: Promise<{ mode?: string | string[] }> }

export default async function FailureScenesPage({ searchParams }: PageProps) {
  const mode = resolveMode((await searchParams).mode)
  return <><header className="page-heading"><div><p className="eyebrow">Reliability proofs</p><h1>Failure scenes</h1></div><p>{mode === "demo" ? "Deterministic event vocabulary linked to representative executions." : "Engine invariants remain explanatory until matching current Live evidence exists."}</p></header><section className="failure-scenes" aria-label="Failure proof scenes">{failureScenes.map((scene) => <FailureSceneCard key={scene.id} scene={scene} mode={mode} />)}</section></>
}