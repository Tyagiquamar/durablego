# DurableGo Deployment

## Hosted demo

The public demo is a single self-contained container on free-tier hosting,
with the read-only proof dashboard on Vercel.

| Component | Runtime | Public URL |
|---|---|---|
| Engine: API, scheduler, two workers, demo traffic driver | Render free tier, built from `Dockerfile.demo` | https://durablego.onrender.com |
| PostgreSQL: workflow state, activity leases, idempotency, event history | Neon free Postgres (`durablego` database) | set via `DURABLEGO_DATABASE_URL` |
| Proof dashboard | Vercel | https://durablego-dashboard.vercel.app |

Verified 2026-08-22: `GET https://durablego.onrender.com/healthz` returns `200`.

### How the single container works

`Dockerfile.demo` builds four binaries (`api`, `scheduler`, `worker`,
`demo-driver`) into an Alpine image. `docker/allinone-entrypoint.sh` starts
the API, scheduler, two independently identified workers (`worker-a`,
`worker-b`), and a continuous demo traffic driver in one process tree.
Leases and fencing tokens are coordinated through PostgreSQL, so the
claim/fencing/recovery proofs behave identically to separate services. If any
component exits, the container exits and the platform restarts everything
together.

Free-tier instances sleep when idle; a cold start can take up to ~60 seconds.
The dashboard defaults to live mode and retries through the wake window. It
never substitutes fixture data for an unavailable or empty API response —
unavailable reads stay visibly unavailable.

## Required variables

| Surface | Variables |
|---|---|
| Render service | `DURABLEGO_DATABASE_URL` (Neon connection string with `sslmode=require`), `DURABLEGO_API_KEY` (protects mutation endpoints), optional `DEMO_DRIVER=false` to disable self-generated traffic |
| Vercel dashboard | `DURABLEGO_API_URL=https://durablego.onrender.com` (server-side only; the browser never sees it) |

The platform-assigned `PORT` is honored by the API automatically when
`DURABLEGO_ADDR` is unset. The dashboard reads workflows and health through
its server-side BFF, so it needs no API key.

Use a long random value for `DURABLEGO_API_KEY` and send
`Authorization: Bearer <key>` when invoking protected endpoints from scripts.

## Reproducing the deploy

1. Create a Neon database and apply `migrations/001_init.sql`
   (e.g. `psql "$DURABLEGO_DATABASE_URL" -f migrations/001_init.sql`).
2. In Render: New Web Service → this repository, Runtime **Docker**,
   Dockerfile path `./Dockerfile.demo`, health check path `/healthz`. Set the
   environment variables in the create form — they do not carry over if the
   service is deleted and recreated.
3. Confirm `GET https://<service>.onrender.com/healthz` responds with `200`.
4. On Vercel: import `web/`, set `DURABLEGO_API_URL` as a server-side
   environment variable, deploy, and create a workflow with an authorized API
   request to verify it appears in the dashboard.

The multi-service layout (separate API, scheduler, and worker containers) is
available via `docker-compose.yml` and the root `Dockerfile`; the all-in-one
image exists because free-tier hosts provide one process per service.

## Local stack

```bash
docker compose up
```

Then start a workflow:

```bash
curl -X POST http://localhost:8080/v1/workflows \
  -H "content-type: application/json" \
  -d '{"namespace":"production","name":"order-processing","idempotency_key":"order-129331","activities":[{"name":"validate","task_queue":"default"}]}'
```

See the repository README for the full local walkthrough.
