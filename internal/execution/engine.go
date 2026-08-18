package execution

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type WorkflowStatus string

const (
	WorkflowRunning   WorkflowStatus = "running"
	WorkflowCompleted WorkflowStatus = "completed"
	WorkflowFailed    WorkflowStatus = "failed"
	WorkflowCancelled WorkflowStatus = "cancelled"
)

type ActivityStatus string

const (
	ActivityReady        ActivityStatus = "ready"
	ActivityRunning      ActivityStatus = "running"
	ActivityRetryPending ActivityStatus = "retry_pending"
	ActivityCompleted    ActivityStatus = "completed"
	ActivityFailed       ActivityStatus = "failed"
	ActivityCancelled    ActivityStatus = "cancelled"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrNoRunnableActivity = errors.New("no runnable activity")
	ErrInvalidTransition  = errors.New("invalid transition")
	ErrStaleLease         = errors.New("stale lease")
	ErrDuplicateClaim     = errors.New("activity already claimed")
)

type ActivityDefinition struct {
	Name         string        `json:"name"`
	TaskQueue    string        `json:"task_queue"`
	DependsOn    []string      `json:"depends_on"`
	MaxAttempts  int           `json:"max_attempts"`
	RetryBackoff time.Duration `json:"retry_backoff"`
}

type WorkflowDefinition struct {
	Name       string               `json:"name"`
	Namespace  string               `json:"namespace"`
	Activities []ActivityDefinition `json:"activities"`
}

type Workflow struct {
	ID             string
	Namespace      string
	Name           string
	IdempotencyKey string
	Status         WorkflowStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Activity struct {
	ID              string
	WorkflowID      string
	Name            string
	TaskQueue       string
	Status          ActivityStatus
	Attempt         int
	MaxAttempts     int
	LeaseOwner      string
	LeaseExpiresAt  time.Time
	FencingToken    int64
	NextAttemptAt   time.Time
	DependsOn       []string
	LastError       string
	CompletedAt     time.Time
	RetryBackoff    time.Duration
	ApplicationKey  string
	SideEffectCount int
}

type Event struct {
	Sequence   int64
	WorkflowID string
	ActivityID string
	Type       string
	Message    string
	At         time.Time
}

type Claim struct {
	ActivityID     string
	WorkflowID     string
	Name           string
	Attempt        int
	FencingToken   int64
	LeaseExpiresAt time.Time
	ApplicationKey string
}

type Engine struct {
	mu           sync.Mutex
	now          func() time.Time
	leaseTTL     time.Duration
	maxAttempts  int
	nextWorkflow int64
	nextActivity int64
	nextEvent    int64
	workflows    map[string]*Workflow
	activities   map[string]*Activity
	activityByWF map[string][]string
	events       map[string][]Event
	idempotency  map[string]string
}

func New(leaseTTL time.Duration, maxAttempts int) *Engine {
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Second
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Engine{
		now:          time.Now,
		leaseTTL:     leaseTTL,
		maxAttempts:  maxAttempts,
		workflows:    map[string]*Workflow{},
		activities:   map[string]*Activity{},
		activityByWF: map[string][]string{},
		events:       map[string][]Event{},
		idempotency:  map[string]string{},
	}
}

func (e *Engine) SetClock(now func() time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.now = now
}

func (e *Engine) Start(def WorkflowDefinition, idempotencyKey string) (*Workflow, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if def.Namespace == "" || def.Name == "" || len(def.Activities) == 0 {
		return nil, false, fmt.Errorf("%w: namespace, name, and activities are required", ErrInvalidTransition)
	}
	idemKey := def.Namespace + "\x00" + idempotencyKey
	if idempotencyKey != "" {
		if existingID, ok := e.idempotency[idemKey]; ok {
			return cloneWorkflow(e.workflows[existingID]), true, nil
		}
	}

	e.nextWorkflow++
	now := e.now()
	workflow := &Workflow{
		ID:             fmt.Sprintf("wf_%06d", e.nextWorkflow),
		Namespace:      def.Namespace,
		Name:           def.Name,
		IdempotencyKey: idempotencyKey,
		Status:         WorkflowRunning,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	e.workflows[workflow.ID] = workflow
	if idempotencyKey != "" {
		e.idempotency[idemKey] = workflow.ID
	}
	e.appendEventLocked(workflow.ID, "", "workflow.started", "workflow created")

	names := map[string]bool{}
	for _, activity := range def.Activities {
		if activity.Name == "" {
			return nil, false, fmt.Errorf("%w: activity name required", ErrInvalidTransition)
		}
		if names[activity.Name] {
			return nil, false, fmt.Errorf("%w: duplicate activity %s", ErrInvalidTransition, activity.Name)
		}
		names[activity.Name] = true
	}
	for _, activity := range def.Activities {
		for _, dep := range activity.DependsOn {
			if !names[dep] {
				return nil, false, fmt.Errorf("%w: unknown dependency %s", ErrInvalidTransition, dep)
			}
		}
		e.nextActivity++
		status := ActivityReady
		if len(activity.DependsOn) > 0 {
			status = ActivityRetryPending
		}
		maxAttempts := activity.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = e.maxAttempts
		}
		backoff := activity.RetryBackoff
		if backoff <= 0 {
			backoff = time.Second
		}
		row := &Activity{
			ID:            fmt.Sprintf("act_%06d", e.nextActivity),
			WorkflowID:    workflow.ID,
			Name:          activity.Name,
			TaskQueue:     activity.TaskQueue,
			Status:        status,
			MaxAttempts:   maxAttempts,
			DependsOn:     append([]string(nil), activity.DependsOn...),
			RetryBackoff:  backoff,
			NextAttemptAt: now,
		}
		e.activities[row.ID] = row
		e.activityByWF[workflow.ID] = append(e.activityByWF[workflow.ID], row.ID)
		e.appendEventLocked(workflow.ID, row.ID, "activity.scheduled", fmt.Sprintf("%s scheduled", row.Name))
	}
	e.releaseReadyLocked(workflow.ID)
	return cloneWorkflow(workflow), false, nil
}

func (e *Engine) Claim(workerID, taskQueue string) (Claim, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()
	ids := e.sortedActivityIDsLocked()
	for _, id := range ids {
		activity := e.activities[id]
		if activity.Status != ActivityReady {
			continue
		}
		if taskQueue != "" && activity.TaskQueue != "" && taskQueue != activity.TaskQueue {
			continue
		}
		activity.Status = ActivityRunning
		activity.Attempt++
		activity.LeaseOwner = workerID
		activity.FencingToken++
		activity.LeaseExpiresAt = now.Add(e.leaseTTL)
		e.touchWorkflowLocked(activity.WorkflowID)
		e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.claimed", fmt.Sprintf("%s claimed by %s token %d", activity.Name, workerID, activity.FencingToken))
		return Claim{
			ActivityID:     activity.ID,
			WorkflowID:     activity.WorkflowID,
			Name:           activity.Name,
			Attempt:        activity.Attempt,
			FencingToken:   activity.FencingToken,
			LeaseExpiresAt: activity.LeaseExpiresAt,
			ApplicationKey: activity.WorkflowID + ":" + activity.Name,
		}, nil
	}
	return Claim{}, ErrNoRunnableActivity
}

func (e *Engine) Heartbeat(activityID, workerID string, token int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	activity, err := e.activityLocked(activityID)
	if err != nil {
		return err
	}
	if !activity.currentLease(workerID, token) {
		e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.stale_heartbeat_rejected", "heartbeat rejected by fencing token")
		return ErrStaleLease
	}
	activity.LeaseExpiresAt = e.now().Add(e.leaseTTL)
	e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.heartbeat", fmt.Sprintf("lease extended by %s", workerID))
	return nil
}

func (e *Engine) Complete(activityID, workerID string, token int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	activity, err := e.activityLocked(activityID)
	if err != nil {
		return err
	}
	if !activity.currentLease(workerID, token) {
		e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.stale_completion_rejected", "completion rejected by fencing token")
		return ErrStaleLease
	}
	activity.Status = ActivityCompleted
	activity.LeaseOwner = ""
	activity.CompletedAt = e.now()
	e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.completed", fmt.Sprintf("%s completed", activity.Name))
	e.releaseReadyLocked(activity.WorkflowID)
	e.completeWorkflowIfDoneLocked(activity.WorkflowID)
	return nil
}

func (e *Engine) Fail(activityID, workerID string, token int64, retryable bool, message string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	activity, err := e.activityLocked(activityID)
	if err != nil {
		return err
	}
	if !activity.currentLease(workerID, token) {
		e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.stale_failure_rejected", "failure rejected by fencing token")
		return ErrStaleLease
	}
	activity.LastError = message
	activity.LeaseOwner = ""
	if retryable && activity.Attempt < activity.MaxAttempts {
		activity.Status = ActivityRetryPending
		activity.NextAttemptAt = e.now().Add(activity.RetryBackoff * time.Duration(activity.Attempt))
		e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.retry_scheduled", message)
		return nil
	}
	activity.Status = ActivityFailed
	e.workflows[activity.WorkflowID].Status = WorkflowFailed
	e.touchWorkflowLocked(activity.WorkflowID)
	e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.failed", message)
	e.appendEventLocked(activity.WorkflowID, "", "workflow.failed", fmt.Sprintf("activity %s failed", activity.Name))
	return nil
}

func (e *Engine) Cancel(workflowID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	workflow, ok := e.workflows[workflowID]
	if !ok {
		return ErrNotFound
	}
	if workflow.Status != WorkflowRunning {
		return fmt.Errorf("%w: workflow is %s", ErrInvalidTransition, workflow.Status)
	}
	workflow.Status = WorkflowCancelled
	workflow.UpdatedAt = e.now()
	for _, id := range e.activityByWF[workflowID] {
		activity := e.activities[id]
		if activity.Status == ActivityReady || activity.Status == ActivityRetryPending {
			activity.Status = ActivityCancelled
			e.appendEventLocked(workflowID, activity.ID, "activity.cancelled", "pending activity cancelled")
		}
	}
	e.appendEventLocked(workflowID, "", "workflow.cancelled", "workflow cancelled")
	return nil
}

func (e *Engine) RunSchedulerPass() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()
	changed := 0
	for _, id := range e.sortedActivityIDsLocked() {
		activity := e.activities[id]
		switch {
		case activity.Status == ActivityRunning && !activity.LeaseExpiresAt.IsZero() && !activity.LeaseExpiresAt.After(now):
			activity.LeaseOwner = ""
			if activity.Attempt < activity.MaxAttempts {
				activity.Status = ActivityReady
				e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.lease_expired", "lease expired and activity became ready")
			} else {
				activity.Status = ActivityFailed
				e.workflows[activity.WorkflowID].Status = WorkflowFailed
				e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.lease_expired_failed", "lease expired and attempts exhausted")
			}
			changed++
		case activity.Status == ActivityRetryPending && !activity.NextAttemptAt.After(now) && e.dependenciesCompleteLocked(activity):
			activity.Status = ActivityReady
			e.appendEventLocked(activity.WorkflowID, activity.ID, "activity.ready", "activity became ready")
			changed++
		}
	}
	return changed
}

func (e *Engine) ListWorkflows() []Workflow {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Workflow, 0, len(e.workflows))
	for _, workflow := range e.workflows {
		out = append(out, *cloneWorkflow(workflow))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (e *Engine) Workflow(id string) (*Workflow, []Activity, []Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	workflow, ok := e.workflows[id]
	if !ok {
		return nil, nil, nil, ErrNotFound
	}
	activities := make([]Activity, 0, len(e.activityByWF[id]))
	for _, activityID := range e.activityByWF[id] {
		activities = append(activities, *cloneActivity(e.activities[activityID]))
	}
	events := append([]Event(nil), e.events[id]...)
	return cloneWorkflow(workflow), activities, events, nil
}

func (e *Engine) activityLocked(id string) (*Activity, error) {
	activity, ok := e.activities[id]
	if !ok {
		return nil, ErrNotFound
	}
	if activity.Status != ActivityRunning {
		return nil, fmt.Errorf("%w: activity is %s", ErrInvalidTransition, activity.Status)
	}
	return activity, nil
}

func (e *Engine) releaseReadyLocked(workflowID string) {
	for _, id := range e.activityByWF[workflowID] {
		activity := e.activities[id]
		if activity.Status != ActivityRetryPending {
			continue
		}
		if activity.NextAttemptAt.After(e.now()) {
			continue
		}
		if e.dependenciesCompleteLocked(activity) {
			activity.Status = ActivityReady
			e.appendEventLocked(workflowID, activity.ID, "activity.ready", fmt.Sprintf("%s became ready", activity.Name))
		}
	}
}

func (e *Engine) dependenciesCompleteLocked(activity *Activity) bool {
	for _, dep := range activity.DependsOn {
		found := false
		for _, id := range e.activityByWF[activity.WorkflowID] {
			upstream := e.activities[id]
			if upstream.Name == dep {
				found = true
				if upstream.Status != ActivityCompleted {
					return false
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (e *Engine) completeWorkflowIfDoneLocked(workflowID string) {
	workflow := e.workflows[workflowID]
	if workflow.Status != WorkflowRunning {
		return
	}
	for _, id := range e.activityByWF[workflowID] {
		if e.activities[id].Status != ActivityCompleted {
			return
		}
	}
	workflow.Status = WorkflowCompleted
	e.touchWorkflowLocked(workflowID)
	e.appendEventLocked(workflowID, "", "workflow.completed", "all activities completed")
}

func (e *Engine) touchWorkflowLocked(workflowID string) {
	if workflow := e.workflows[workflowID]; workflow != nil {
		workflow.UpdatedAt = e.now()
	}
}

func (e *Engine) appendEventLocked(workflowID, activityID, typ, message string) {
	e.nextEvent++
	e.events[workflowID] = append(e.events[workflowID], Event{
		Sequence:   e.nextEvent,
		WorkflowID: workflowID,
		ActivityID: activityID,
		Type:       typ,
		Message:    message,
		At:         e.now(),
	})
}

func (e *Engine) sortedActivityIDsLocked() []string {
	ids := make([]string, 0, len(e.activities))
	for id := range e.activities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (a *Activity) currentLease(workerID string, token int64) bool {
	return a.Status == ActivityRunning && a.LeaseOwner == workerID && a.FencingToken == token
}

func cloneWorkflow(w *Workflow) *Workflow {
	cp := *w
	return &cp
}

func cloneActivity(a *Activity) *Activity {
	cp := *a
	cp.DependsOn = append([]string(nil), a.DependsOn...)
	return &cp
}
