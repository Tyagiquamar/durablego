FROM golang:1.26-alpine AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/durablego-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/durablego-scheduler ./cmd/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/durablego-worker ./cmd/worker

FROM node:24-alpine AS dashboard-builder

WORKDIR /src
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN corepack enable && corepack prepare pnpm@11.8.0 --activate && pnpm install --frozen-lockfile
COPY web/ .
RUN pnpm build

FROM node:24-alpine AS dashboard

WORKDIR /app
COPY --from=dashboard-builder /src/.next/standalone ./
COPY --from=dashboard-builder /src/.next/static ./.next/static
EXPOSE 3000
ENTRYPOINT ["node", "server.js"]

FROM alpine:3.21 AS railway

RUN apk --no-cache add ca-certificates
COPY --from=go-builder /bin/durablego-api /durablego-api
COPY --from=go-builder /bin/durablego-scheduler /durablego-scheduler
COPY --from=go-builder /bin/durablego-worker /durablego-worker
COPY docker/railway-entrypoint.sh /railway-entrypoint.sh
RUN chmod +x /railway-entrypoint.sh
ENTRYPOINT ["/railway-entrypoint.sh"]