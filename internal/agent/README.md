# Astra Agent

`astra-agent` is the node-side component of `astra-scheduler`.

The central scheduler decides where a workload should run and writes an
`AIResourceAllocation`. The agent watches allocations assigned to its node,
turns them into local plans, applies those plans through a backend, and reports
actual node state back to Kubernetes.

The agent is not a second independent cluster scheduler. It should align with
the central scheduler's placement decision and only perform node-local resource
realization. In other words, the central scheduler decides whether a workload
can be placed on this node, while the agent decides how the accepted
allocations share the node's local AI resources at the current moment.

## Current Responsibilities

The current agent version focuses on proving that the scheduling chain can run
end to end, even when the backend is fake.

Current flow:

```text
AIResourceAllocation
  -> astra-agent watch event
  -> list all allocations on this node
  -> build local plans
  -> backend.Apply
  -> update AIResourceAllocation.status
  -> backend.Snapshot
  -> update AINodeResourceProfile.status
```

The fake backend does not operate real GPU devices. It simulates resource
accounting and status updates so that the scheduler-agent control loop can be
tested without a GPU cluster.

## Implemented Local Scheduling Baseline

The local scheduler currently implements a thin baseline:

- orders allocations by workload priority;
- within the same priority, handles active time-window workloads first;
- for non-active workloads at the same priority, preserves deployment order by
  processing older allocations first;
- observes local resource snapshots through resource-manager plugins;
- validates GPU, KV cache, and token-throughput requirements through
  resource-manager plugins;
- runs cross-resource constraints;
- selects fallback actions through an `ActionPolicy`;
- creates local plans with actions such as `apply`, `throttle`, `pause`, and
  `evict`;
- falls back to an allowed downgrade action when desired resources cannot fit.

The current allocation policy follows the intended two-level scheduling
contract:

- higher-priority workloads are planned before lower-priority workloads;
- within the same priority, workloads in their active peak window receive their
  `max` budget first;
- same-priority workloads outside the active window keep `required` resources
  during another same-priority workload's peak window, matching the central
  scheduler's time-window envelope check;
- lower-priority workloads use whatever remains after higher-priority groups
  have been planned.

This is enough to validate the control path and the local resource-accounting
model, but it is not yet a full production node-local scheduler.

## Extension Points

The current agent architecture is intentionally modular:

```text
agent controller
  -> local scheduler
      -> resource managers
      -> action policy
      -> local constraints
      -> resource ledger
      -> plans
  -> backend
  -> status recorder
```

The main extension points are:

- `ResourceManager`: observes and validates one resource dimension, such as GPU,
  KV cache, token throughput, or future resources.
- `ActionPolicy`: chooses the normal action and fallback action for an
  allocation, such as apply, throttle, pause, or evict.
- `Constraint`: checks cross-resource or backend capability constraints after a
  local plan is built.
- `Backend`: applies a plan using fake state, NVIDIA/DCGM, runtime APIs, DRA,
  CRI, or another future integration.
- `statusRecorder`: centralizes allocation status transitions so the controller
  does not own the whole allocation state machine.

This mirrors the central scheduler design: hard constraints, soft policies,
resource-specific managers, and backend-specific execution are kept separate.

## Near-term Completion Goals

The next two features are the most important ones for making the prototype feel
complete.

### 1. Node-local Resource Ledger

The agent should maintain a local resource ledger for each reconcile cycle.

The ledger should:

- start from observed node capacity;
- allocate required resources from high priority to low priority;
- allocate preferred resources when possible;
- track remaining GPU, GPU memory, KV cache, and token throughput;
- record which allocation received required, preferred, or degraded resources;
- make resource decisions deterministic and explainable.

This turns the agent from a validator into a real node-local scheduler.

The ledger should always start from an observed backend snapshot. Plans should
be derived from the latest local state, marked as pending/applying through
`AIResourceAllocation.status`, and then committed after `backend.Apply`
succeeds. This keeps local decisions idempotent and makes node drift visible to
the central scheduler.

### 2. Richer Plan Semantics

The current `Plan` object only contains an allocation, an action, and a reason.
It should be extended to describe the desired local transition.

Future plan fields should include:

- action: `apply`, `throttle`, `pause`, `resume`, or `evict`;
- desired resources;
- actual resources to apply;
- previous phase and target phase;
- priority;
- demand shape;
- degradation reason;
- whether the allocation received required or preferred resources.

This gives the backend enough information to update CR status clearly and, in a
future real backend, to call the correct runtime or Kubernetes operation.

## Future Work

The following features are intentionally left for later research or production
versions.

### Richer Local Action Policy

The current fallback order is simple and implemented as `ActionPolicy`. A future
policy should decide:

- how much to throttle;
- which low-priority workload to pause first;
- when eviction is justified;
- how to protect high-priority required resources;
- how to let low-priority workloads borrow idle capacity.

### Resume and Expansion

The agent should not only shrink workloads under pressure. It should also:

- resume paused workloads when pressure drops;
- expand throttled workloads back toward preferred resources;
- restore low-priority borrowed capacity during off-peak periods.

### Allocation State Machine

`AIResourceAllocation.status.phase` should eventually represent a richer
lifecycle, for example:

```text
Pending
Applied
Throttled
Paused
Resuming
Evicted
Rejected
Degraded
Released
```

The state machine should make reconcile actions idempotent and easy to debug.

### Idempotency and Release Handling

Because the agent is a Kubernetes controller, every operation may be retried.
Future implementations should guarantee that:

- repeated apply does not double count resources;
- repeated throttle does not keep reducing the budget;
- repeated pause/resume is safe;
- deleted allocations release resources;
- node restart and agent restart can rebuild state from Kubernetes objects.

### Real Backend Integration

The current `real` backend is a feasibility skeleton. Future backends can
integrate with:

- NVML, DCGM, and NVIDIA device plugin state;
- MIG observation and controlled reconfiguration;
- vLLM, SGLang, TGI, Triton, or gateway APIs for concurrency and token control;
- Kubernetes eviction/delete APIs;
- CRI/containerd or Dynamic Resource Allocation where appropriate.

The first production-like goal should be read-only observation, followed by
carefully guarded write actions.

### Conflict Protocol with the Central Scheduler

The central scheduler may place a workload on a node, but the agent may later
observe that the local state has changed. Future versions should define a clear
protocol:

```text
central allocation accepted
  -> agent applied
  -> agent degraded
  -> agent rejected
  -> scheduler reschedules or triggers reclaim
```

This is the key boundary between cluster-level scheduling and node-local
arbitration.

The guiding principle is that the agent should not routinely overturn central
placement. If local state drift makes a central allocation impossible to apply,
the agent should report the failed or degraded state clearly through status so
the central scheduler can reclaim, reschedule, or adjust policy.
