package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tyagiquamar/durablego/internal/execution"
)

type Postgres struct {
	pool        *pgxpool.Pool
	leaseTTL    time.Duration
	maxAttempts int
}

func NewPostgres(ctx context.Context, databaseURL string, leaseTTL time.Duration, maxAttempts int) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Second
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Postgres{pool: pool, leaseTTL: leaseTTL, maxAttempts: maxAttempts}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) Start(def execution.WorkflowDefinition, idempotencyKey string) (*execution.Workflow, bool, error) {
	ctx := context.Background()
	if def.Namespace == "" || def.Name == "" || len(def.Activities) == 0 {
		return nil, false, fmt.Errorf("%w: namespace, name, and activities are required", execution.ErrInvalidTransition)
	}
	names := map[string]bool{}
	for _, activity := range def.Activities {
		if activity.Name == "" {
			return nil, false, fmt.Errorf("%w: activity name required", execution.ErrInvalidTransition)
		}
		if names[activity.Name] {
			return nil, false, fmt.Errorf("%w: duplicate activity %s", execution.ErrInvalidTransition, activity.Name)
		}
		names[activity.Name] = true
	}
	for _, activity := range def.Activities {
		for _, dep := range activity.DependsOn {
			if !names[dep] {
				return nil, false, fmt.Errorf("%w: unknown dependency %s", execution.ErrInvalidTransition, dep)
			}
		}
	}

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, err
	}
	defer rollback(ctx, tx)

	namespaceID, err := ensureNamespace(ctx, tx, def.Namespace)
	if err != nil {
		return nil, false, err
	}
	if idempotencyKey != "" {
		var workflowID string
		err := tx.QueryRow(ctx, `
			SELECT workflow_execution_id::text
			FROM idempotency_keys
			WHERE namespace_id = $1 AND key = $2
		`, namespaceID, idempotencyKey).Scan(&workflowID)
		if err == nil {
			workflow, _, _, err := p.workflowTx(ctx, tx, workflowID)
			if err != nil {
				return nil, false, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, false, err
			}
			return workflow, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
	}

	var definitionID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO workflow_definitions(namespace_id, name)
		VALUES ($1, $2)
		ON CONFLICT(namespace_id, name, version) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, namespaceID, def.Name).Scan(&definitionID); err != nil {
		return nil, false, err
	}

	workflow := &execution.Workflow{
		Namespace:      def.Namespace,
		Name:           def.Name,
		IdempotencyKey: idempotencyKey,
		Status:         execution.WorkflowRunning,
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO workflow_executions(namespace_id, definition_id, status)
		VALUES ($1, $2, $3)
		RETURNING id::text, created_at, updated_at
	`, namespaceID, definitionID, workflow.Status).Scan(&workflow.ID, &workflow.CreatedAt, &workflow.UpdatedAt); err != nil {
		return nil, false, err
	}
	if idempotencyKey != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO idempotency_keys(namespace_id, key, workflow_execution_id)
			VALUES ($1, $2, $3)
		`, namespaceID, idempotencyKey, workflow.ID); err != nil {
			return nil, false, err
		}
	}
	if err := appendEvent(ctx, tx, workflow.ID, "", "workflow.started", "workflow created"); err != nil {
		return nil, false, err
	}
	for _, activity := range def.Activities {
		status := execution.ActivityReady
		if len(activity.DependsOn) > 0 {
			status = execution.ActivityRetryPending
		}
		maxAttempts := activity.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = p.maxAttempts
		}
		backoff := activity.RetryBackoff
		if backoff <= 0 {
			backoff = time.Second
		}
		taskQueue := activity.TaskQueue
		if taskQueue == "" {
			taskQueue = "default"
		}
		var activityID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO activity_executions(workflow_execution_id, name, task_queue, status, max_attempts, retry_backoff_ms)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id::text
		`, workflow.ID, activity.Name, taskQueue, status, maxAttempts, int(backoff.Milliseconds())).Scan(&activityID); err != nil {
			return nil, false, err
		}
		for _, dep := range activity.DependsOn {
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_dependencies(workflow_execution_id, activity_name, depends_on_name)
				VALUES ($1, $2, $3)
			`, workflow.ID, activity.Name, dep); err != nil {
				return nil, false, err
			}
		}
		if err := appendEvent(ctx, tx, workflow.ID, activityID, "activity.scheduled", fmt.Sprintf("%s scheduled", activity.Name)); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return workflow, false, nil
}

func (p *Postgres) Claim(workerID, taskQueue string) (execution.Claim, error) {
	ctx := context.Background()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return execution.Claim{}, err
	}
	defer rollback(ctx, tx)

	leaseSeconds := int64(p.leaseTTL.Seconds())
	var claim execution.Claim
	err = tx.QueryRow(ctx, `
		WITH picked AS (
			SELECT id
			FROM activity_executions
			WHERE status = 'ready'
			  AND ($1 = '' OR task_queue = '' OR task_queue = $1)
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE activity_executions AS a
		SET status = 'running',
		    attempt = attempt + 1,
		    lease_owner = $2,
		    fencing_token = fencing_token + 1,
		    lease_expires_at = now() + make_interval(secs => $3),
		    heartbeat_at = now(),
		    updated_at = now()
		FROM picked
		WHERE a.id = picked.id
		RETURNING a.id::text, a.workflow_execution_id::text, a.name, a.attempt, a.fencing_token, a.lease_expires_at
	`, taskQueue, workerID, leaseSeconds).Scan(&claim.ActivityID, &claim.WorkflowID, &claim.Name, &claim.Attempt, &claim.FencingToken, &claim.LeaseExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return execution.Claim{}, execution.ErrNoRunnableActivity
	}
	if err != nil {
		return execution.Claim{}, err
	}
	claim.ApplicationKey = claim.WorkflowID + ":" + claim.Name
	if err := appendEvent(ctx, tx, claim.WorkflowID, claim.ActivityID, "activity.claimed", fmt.Sprintf("%s claimed by %s token %d", claim.Name, workerID, claim.FencingToken)); err != nil {
		return execution.Claim{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return execution.Claim{}, err
	}
	return claim, nil
}

func (p *Postgres) Heartbeat(activityID, workerID string, token int64) error {
	ctx := context.Background()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	activity, err := lockActivity(ctx, tx, activityID)
	if err != nil {
		return err
	}
	if !currentLease(activity, workerID, token) {
		_ = appendEvent(ctx, tx, activity.WorkflowID, activity.ID, "activity.stale_heartbeat_rejected", "heartbeat rejected by fencing token")
		_ = tx.Commit(ctx)
		return execution.ErrStaleLease
	}
	_, err = tx.Exec(ctx, `
		UPDATE activity_executions
		SET lease_expires_at = now() + make_interval(secs => $2), heartbeat_at = now(), updated_at = now()
		WHERE id = $1
	`, activityID, int64(p.leaseTTL.Seconds()))
	if err != nil {
		return err
	}
	if err := appendEvent(ctx, tx, activity.WorkflowID, activity.ID, "activity.heartbeat", fmt.Sprintf("lease extended by %s", workerID)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Complete(activityID, workerID string, token int64) error {
	ctx := context.Background()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	activity, err := lockActivity(ctx, tx, activityID)
	if err != nil {
		return err
	}
	if !currentLease(activity, workerID, token) {
		_ = appendEvent(ctx, tx, activity.WorkflowID, activity.ID, "activity.stale_completion_rejected", "completion rejected by fencing token")
		_ = tx.Commit(ctx)
		return execution.ErrStaleLease
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity_executions
		SET status = 'completed', lease_owner = NULL, updated_at = now()
		WHERE id = $1
	`, activityID); err != nil {
		return err
	}
	if err := appendEvent(ctx, tx, activity.WorkflowID, activity.ID, "activity.completed", fmt.Sprintf("%s completed", activity.Name)); err != nil {
		return err
	}
	if err := releaseReady(ctx, tx, activity.WorkflowID); err != nil {
		return err
	}
	if err := completeWorkflowIfDone(ctx, tx, activity.WorkflowID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Fail(activityID, workerID string, token int64, retryable bool, message string) error {
	ctx := context.Background()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	activity, err := lockActivity(ctx, tx, activityID)
	if err != nil {
		return err
	}
	if !currentLease(activity, workerID, token) {
		_ = appendEvent(ctx, tx, activity.WorkflowID, activity.ID, "activity.stale_failure_rejected", "failure rejected by fencing token")
		_ = tx.Commit(ctx)
		return execution.ErrStaleLease
	}
	if retryable && activity.Attempt < activity.MaxAttempts {
		backoff := activity.RetryBackoff * time.Duration(activity.Attempt)
		_, err := tx.Exec(ctx, `
			UPDATE activity_executions
			SET status = 'retry_pending',
			    lease_owner = NULL,
			    next_attempt_at = now() + make_interval(secs => $2),
			    last_error = $3,
			    updated_at = now()
			WHERE id = $1
		`, activityID, int64(backoff.Seconds()), message)
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, activity.WorkflowID, activity.ID, "activity.retry_scheduled", message); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity_executions
		SET status = 'failed', lease_owner = NULL, last_error = $2, updated_at = now()
		WHERE id = $1
	`, activityID, message); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_executions
		SET status = 'failed', updated_at = now()
		WHERE id = $1
	`, activity.WorkflowID); err != nil {
		return err
	}
	if err := appendEvent(ctx, tx, activity.WorkflowID, activity.ID, "activity.failed", message); err != nil {
		return err
	}
	if err := appendEvent(ctx, tx, activity.WorkflowID, "", "workflow.failed", fmt.Sprintf("activity %s failed", activity.Name)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Cancel(workflowID string) error {
	ctx := context.Background()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	var status execution.WorkflowStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM workflow_executions WHERE id = $1 FOR UPDATE`, workflowID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return execution.ErrNotFound
	} else if err != nil {
		return err
	}
	if status != execution.WorkflowRunning {
		return fmt.Errorf("%w: workflow is %s", execution.ErrInvalidTransition, status)
	}
	if _, err := tx.Exec(ctx, `UPDATE workflow_executions SET status = 'cancelled', updated_at = now() WHERE id = $1`, workflowID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		UPDATE activity_executions
		SET status = 'cancelled', updated_at = now()
		WHERE workflow_execution_id = $1 AND status IN ('ready', 'retry_pending')
		RETURNING id::text
	`, workflowID)
	if err != nil {
		return err
	}
	var cancelled []string
	for rows.Next() {
		var activityID string
		if err := rows.Scan(&activityID); err != nil {
			rows.Close()
			return err
		}
		cancelled = append(cancelled, activityID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, activityID := range cancelled {
		if err := appendEvent(ctx, tx, workflowID, activityID, "activity.cancelled", "pending activity cancelled"); err != nil {
			return err
		}
	}
	if err := appendEvent(ctx, tx, workflowID, "", "workflow.cancelled", "workflow cancelled"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RunSchedulerPass() int {
	ctx := context.Background()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0
	}
	defer rollback(ctx, tx)

	changed := 0
	rows, err := tx.Query(ctx, `
		UPDATE activity_executions
		SET status = CASE WHEN attempt < max_attempts THEN 'ready' ELSE 'failed' END,
		    lease_owner = NULL,
		    updated_at = now()
		WHERE status = 'running' AND lease_expires_at <= now()
		RETURNING id::text, workflow_execution_id::text, name, status
	`)
	if err != nil {
		return 0
	}
	type expiredActivity struct {
		activityID string
		workflowID string
		name       string
		status     execution.ActivityStatus
	}
	var expired []expiredActivity
	for rows.Next() {
		var row expiredActivity
		if err := rows.Scan(&row.activityID, &row.workflowID, &row.name, &row.status); err != nil {
			rows.Close()
			return 0
		}
		expired = append(expired, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0
	}
	for _, row := range expired {
		eventType := "activity.lease_expired"
		message := "lease expired and activity became ready"
		if row.status == execution.ActivityFailed {
			eventType = "activity.lease_expired_failed"
			message = "lease expired and attempts exhausted"
			if _, err := tx.Exec(ctx, `UPDATE workflow_executions SET status = 'failed', updated_at = now() WHERE id = $1`, row.workflowID); err != nil {
				return 0
			}
			if err := appendEvent(ctx, tx, row.workflowID, "", "workflow.failed", fmt.Sprintf("activity %s failed", row.name)); err != nil {
				return 0
			}
		}
		if err := appendEvent(ctx, tx, row.workflowID, row.activityID, eventType, message); err != nil {
			return 0
		}
		changed++
	}
	if err := releaseReady(ctx, tx, ""); err != nil {
		return 0
	}
	if err := tx.Commit(ctx); err != nil {
		return 0
	}
	return changed
}

func (p *Postgres) ListWorkflows() ([]execution.Workflow, error) {
	ctx := context.Background()
	rows, err := p.pool.Query(ctx, `
		SELECT w.id::text, n.name, d.name, COALESCE(i.key, ''), w.status, w.created_at, w.updated_at
		FROM workflow_executions w
		JOIN namespaces n ON n.id = w.namespace_id
		JOIN workflow_definitions d ON d.id = w.definition_id
		LEFT JOIN idempotency_keys i ON i.workflow_execution_id = w.id
		ORDER BY w.created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workflows []execution.Workflow
	for rows.Next() {
		var workflow execution.Workflow
		if err := rows.Scan(&workflow.ID, &workflow.Namespace, &workflow.Name, &workflow.IdempotencyKey, &workflow.Status, &workflow.CreatedAt, &workflow.UpdatedAt); err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if workflows == nil {
		workflows = []execution.Workflow{}
	}
	return workflows, nil
}

func (p *Postgres) Workflow(id string) (*execution.Workflow, []execution.Activity, []execution.Event, error) {
	ctx := context.Background()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, nil, err
	}
	defer rollback(ctx, tx)
	workflow, activities, events, err := p.workflowTx(ctx, tx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, nil, err
	}
	return workflow, activities, events, nil
}

func (p *Postgres) workflowTx(ctx context.Context, tx pgx.Tx, id string) (*execution.Workflow, []execution.Activity, []execution.Event, error) {
	var workflow execution.Workflow
	err := tx.QueryRow(ctx, `
		SELECT w.id::text, n.name, d.name, COALESCE(i.key, ''), w.status, w.created_at, w.updated_at
		FROM workflow_executions w
		JOIN namespaces n ON n.id = w.namespace_id
		JOIN workflow_definitions d ON d.id = w.definition_id
		LEFT JOIN idempotency_keys i ON i.workflow_execution_id = w.id
		WHERE w.id = $1
	`, id).Scan(&workflow.ID, &workflow.Namespace, &workflow.Name, &workflow.IdempotencyKey, &workflow.Status, &workflow.CreatedAt, &workflow.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil, execution.ErrNotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}

	activities, err := loadActivities(ctx, tx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	events, err := loadEvents(ctx, tx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	return &workflow, activities, events, nil
}

func ensureNamespace(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO namespaces(name)
		VALUES ($1)
		ON CONFLICT(name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name).Scan(&id)
	return id, err
}

func appendEvent(ctx context.Context, tx pgx.Tx, workflowID, activityID, eventType, message string) error {
	var nullableActivity any
	if activityID != "" {
		nullableActivity = activityID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO workflow_events(workflow_execution_id, activity_execution_id, type, message)
		VALUES ($1, $2, $3, $4)
	`, workflowID, nullableActivity, eventType, message)
	return err
}

func lockActivity(ctx context.Context, tx pgx.Tx, activityID string) (execution.Activity, error) {
	var activity execution.Activity
	var leaseOwner *string
	err := tx.QueryRow(ctx, `
		SELECT id::text, workflow_execution_id::text, name, task_queue, status, attempt, max_attempts,
		       retry_backoff_ms, lease_owner, lease_expires_at, fencing_token, next_attempt_at, COALESCE(last_error, '')
		FROM activity_executions
		WHERE id = $1
		FOR UPDATE
	`, activityID).Scan(
		&activity.ID,
		&activity.WorkflowID,
		&activity.Name,
		&activity.TaskQueue,
		&activity.Status,
		&activity.Attempt,
		&activity.MaxAttempts,
		&activity.RetryBackoff,
		&leaseOwner,
		&activity.LeaseExpiresAt,
		&activity.FencingToken,
		&activity.NextAttemptAt,
		&activity.LastError,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return execution.Activity{}, execution.ErrNotFound
	}
	if err != nil {
		return execution.Activity{}, err
	}
	if activity.Status != execution.ActivityRunning {
		return execution.Activity{}, fmt.Errorf("%w: activity is %s", execution.ErrInvalidTransition, activity.Status)
	}
	if leaseOwner != nil {
		activity.LeaseOwner = *leaseOwner
	}
	activity.RetryBackoff *= time.Millisecond
	return activity, nil
}

func currentLease(activity execution.Activity, workerID string, token int64) bool {
	return activity.Status == execution.ActivityRunning && activity.LeaseOwner == workerID && activity.FencingToken == token
}

func releaseReady(ctx context.Context, tx pgx.Tx, workflowID string) error {
	args := []any{}
	scope := ""
	if workflowID != "" {
		args = append(args, workflowID)
		scope = "AND a.workflow_execution_id = $1"
	}
	rows, err := tx.Query(ctx, `
		UPDATE activity_executions AS a
		SET status = 'ready', updated_at = now()
		WHERE a.status = 'retry_pending'
		  AND a.next_attempt_at <= now()
		  `+scope+`
		  AND NOT EXISTS (
		    SELECT 1
		    FROM activity_dependencies d
		    JOIN activity_executions dep
		      ON dep.workflow_execution_id = d.workflow_execution_id
		     AND dep.name = d.depends_on_name
		    WHERE d.workflow_execution_id = a.workflow_execution_id
		      AND d.activity_name = a.name
		      AND dep.status <> 'completed'
		  )
		RETURNING a.id::text, a.workflow_execution_id::text, a.name
	`, args...)
	if err != nil {
		return err
	}
	type readyActivity struct {
		activityID string
		workflowID string
		name       string
	}
	var ready []readyActivity
	for rows.Next() {
		var row readyActivity
		if err := rows.Scan(&row.activityID, &row.workflowID, &row.name); err != nil {
			rows.Close()
			return err
		}
		ready = append(ready, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, row := range ready {
		if err := appendEvent(ctx, tx, row.workflowID, row.activityID, "activity.ready", fmt.Sprintf("%s became ready", row.name)); err != nil {
			return err
		}
	}
	return nil
}

func completeWorkflowIfDone(ctx context.Context, tx pgx.Tx, workflowID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE workflow_executions
		SET status = 'completed', updated_at = now()
		WHERE id = $1
		  AND status = 'running'
		  AND NOT EXISTS (
		    SELECT 1 FROM activity_executions
		    WHERE workflow_execution_id = $1 AND status <> 'completed'
		  )
	`, workflowID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return appendEvent(ctx, tx, workflowID, "", "workflow.completed", "all activities completed")
	}
	return nil
}

func loadActivities(ctx context.Context, tx pgx.Tx, workflowID string) ([]execution.Activity, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, workflow_execution_id::text, name, task_queue, status, attempt, max_attempts,
		       retry_backoff_ms, COALESCE(lease_owner, ''), lease_expires_at, fencing_token, next_attempt_at, COALESCE(last_error, '')
		FROM activity_executions
		WHERE workflow_execution_id = $1
		ORDER BY created_at, id
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var activities []execution.Activity
	for rows.Next() {
		var activity execution.Activity
		var leaseExpiresAt *time.Time
		if err := rows.Scan(
			&activity.ID,
			&activity.WorkflowID,
			&activity.Name,
			&activity.TaskQueue,
			&activity.Status,
			&activity.Attempt,
			&activity.MaxAttempts,
			&activity.RetryBackoff,
			&activity.LeaseOwner,
			&leaseExpiresAt,
			&activity.FencingToken,
			&activity.NextAttemptAt,
			&activity.LastError,
		); err != nil {
			return nil, err
		}
		if leaseExpiresAt != nil {
			activity.LeaseExpiresAt = *leaseExpiresAt
		}
		activity.RetryBackoff *= time.Millisecond
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	deps, err := loadDependencies(ctx, tx, workflowID)
	if err != nil {
		return nil, err
	}
	for i := range activities {
		activities[i].DependsOn = deps[activities[i].Name]
	}
	return activities, nil
}

func loadDependencies(ctx context.Context, tx pgx.Tx, workflowID string) (map[string][]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT activity_name, depends_on_name
		FROM activity_dependencies
		WHERE workflow_execution_id = $1
		ORDER BY activity_name, depends_on_name
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deps := map[string][]string{}
	for rows.Next() {
		var activityName, dependsOn string
		if err := rows.Scan(&activityName, &dependsOn); err != nil {
			return nil, err
		}
		deps[activityName] = append(deps[activityName], dependsOn)
	}
	return deps, rows.Err()
}

func loadEvents(ctx context.Context, tx pgx.Tx, workflowID string) ([]execution.Event, error) {
	rows, err := tx.Query(ctx, `
		SELECT sequence, workflow_execution_id::text, COALESCE(activity_execution_id::text, ''), type, message, created_at
		FROM workflow_events
		WHERE workflow_execution_id = $1
		ORDER BY sequence
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []execution.Event
	for rows.Next() {
		var event execution.Event
		if err := rows.Scan(&event.Sequence, &event.WorkflowID, &event.ActivityID, &event.Type, &event.Message, &event.At); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
