# Failure Scenes

## Worker Crash After Claim

Invariant: an activity leased by a dead worker becomes runnable again after the lease expires, and the event history records the recovery.

Steps:

1. Start an order-processing workflow.
2. Poll and claim `inventory` with Worker A.
3. Stop Worker A without completing.
4. Run a scheduler pass after the lease deadline.
5. Poll with Worker B and complete the activity.

Expected event types: `activity.claimed`, `activity.lease_expired`, `activity.claimed`, `activity.completed`.

## Stale Worker Completion

Invariant: a stale fencing token cannot mutate an activity after a newer worker has reclaimed it.

Steps:

1. Worker A claims the activity with token 1.
2. The lease expires.
3. Worker B claims the activity with token 2.
4. Worker A reports completion with token 1.

Expected result: the completion returns a conflict and history includes `activity.stale_completion_rejected`.

## Duplicate Delivery

Invariant: DurableGo is at least once, so side-effecting activity code must provide its own idempotency key.

Steps:

1. Run the payment activity with application idempotency key `workflow_id:payment`.
2. Simulate a crash after the external side effect but before DurableGo completion.
3. Let the scheduler retry the activity.

Expected result: activity execution repeats, while the demo payment ledger suppresses the duplicate external charge by application key.

