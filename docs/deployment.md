# DurableGo Deployment

## Production topology

| Service | Runtime | Purpose |
|---|---|---|
| PostgreSQL | Railway Postgres | Durable workflow state, activity leases, idempotency, and event history |
| API | Railway | Public read API, protected mutation API, health, and metrics |
| Scheduler | Railway | Retry activation and expired-lease recovery |
| Worker A | Railway | Demo activity worker |
| Worker B | Railway | Independent demo activity worker for claim and fencing proofs |
| Dashboard | Vercel | Public, read-only proof surface |

`Dockerfile` produces the Railway runtime by default. Set `DURABLEGO_RUN` on
each Railway service to `api`, `scheduler`, or `worker`. The API uses the
platform-provided `PORT` automatically when `DURABLEGO_ADDR` is unset.

## Required variables

| Service | Variables |
|---|---|
| API | `DURABLEGO_RUN=api`, `DURABLEGO_DATABASE_URL`, `DURABLEGO_API_KEY` |
| Scheduler | `DURABLEGO_RUN=scheduler`, `DURABLEGO_DATABASE_URL` |
| Worker A | `DURABLEGO_RUN=worker`, `DURABLEGO_DATABASE_URL`, `DURABLEGO_WORKER_ID=worker-a` |
| Worker B | `DURABLEGO_RUN=worker`, `DURABLEGO_DATABASE_URL`, `DURABLEGO_WORKER_ID=worker-b` |
| Vercel dashboard | `DURABLEGO_API_URL=https://<api-domain>` |

Use the Railway Postgres connection string for `DURABLEGO_DATABASE_URL`. The
API's `DURABLEGO_API_KEY` protects every mutation endpoint; the dashboard
doesn't need it because it only reads workflow state and health. Use a long,
random secret and include `Authorization: Bearer <key>` when invoking the
protected endpoints from scripts.

## Deploy sequence

1. Create a Railway project and provision Railway Postgres.
2. Create API, scheduler, and two worker services from this repository.
3. Set the variables above, using the Postgres service reference for the
   database URL.
4. Confirm `GET https://<api-domain>/healthz` responds with `200`.
5. Deploy `web/` to Vercel and set `DURABLEGO_API_URL` to the API's public URL.
6. Open the Vercel URL and create a workflow with an authorized API request to
   verify it appears in the dashboard.

## Current hosted-deploy blocker

Railway CLI authentication is present, but the account's Railway trial expired
before a project could be created. No Railway project or billable resource was
created. Enable a Railway plan, then follow the sequence above. RelayDB has
the same account prerequisite; its specific topology is documented in
`relaydb/docs/deployment.md`.