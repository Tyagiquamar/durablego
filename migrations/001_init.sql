CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE namespaces (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workflow_definitions (
  id BIGSERIAL PRIMARY KEY,
  namespace_id BIGINT NOT NULL REFERENCES namespaces(id),
  name TEXT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, name, version)
);

CREATE TABLE workflow_executions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  namespace_id BIGINT NOT NULL REFERENCES namespaces(id),
  definition_id BIGINT NOT NULL REFERENCES workflow_definitions(id),
  status TEXT NOT NULL,
  input JSONB NOT NULL DEFAULT '{}'::jsonb,
  output JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (status IN ('running', 'completed', 'failed', 'cancelled', 'timed_out'))
);

CREATE TABLE idempotency_keys (
  namespace_id BIGINT NOT NULL REFERENCES namespaces(id),
  key TEXT NOT NULL,
  workflow_execution_id UUID NOT NULL REFERENCES workflow_executions(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(namespace_id, key)
);

CREATE TABLE activity_executions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_execution_id UUID NOT NULL REFERENCES workflow_executions(id),
  name TEXT NOT NULL,
  task_queue TEXT NOT NULL DEFAULT 'default',
  status TEXT NOT NULL,
  attempt INT NOT NULL DEFAULT 0,
  max_attempts INT NOT NULL DEFAULT 3,
  retry_backoff_ms INT NOT NULL DEFAULT 1000,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  heartbeat_at TIMESTAMPTZ,
  fencing_token BIGINT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (status IN ('ready', 'running', 'retry_pending', 'completed', 'failed', 'cancelled', 'timed_out'))
);

CREATE TABLE activity_dependencies (
  workflow_execution_id UUID NOT NULL REFERENCES workflow_executions(id),
  activity_name TEXT NOT NULL,
  depends_on_name TEXT NOT NULL,
  PRIMARY KEY(workflow_execution_id, activity_name, depends_on_name)
);

CREATE TABLE workflow_events (
  workflow_execution_id UUID NOT NULL REFERENCES workflow_executions(id),
  sequence BIGSERIAL NOT NULL,
  activity_execution_id UUID,
  type TEXT NOT NULL,
  message TEXT NOT NULL,
  attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(workflow_execution_id, sequence)
);

CREATE INDEX idx_activity_ready ON activity_executions(status, next_attempt_at, task_queue);
CREATE INDEX idx_activity_leases ON activity_executions(status, lease_expires_at);
CREATE INDEX idx_workflow_events_sequence ON workflow_events(workflow_execution_id, sequence);
