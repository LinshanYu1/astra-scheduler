# Astra Scheduler Evaluation Run

- Run ID: `20260617-050846`
- Cluster: 2 control-plane nodes and 2 worker nodes, Kubernetes `v1.36.1`, ARM64
- Backend: fake node-resource backend with real Kubernetes scheduling, CRDs, scheduler plugin, and node agents
- Result: all four scenarios scheduled successfully; all observed allocations reached `Applied`

## Scenario Results

| Scenario | Expected behavior | Observed placement | Allocation result |
|---|---|---|---|
| Resource-shape placement | Decode/KV-heavy workloads prefer decode/KV capacity; prefill-heavy workload prefers prefill capacity | decode and KV workloads -> `worker0`; prefill workload -> `worker1` | 3/3 `Applied` |
| Time-window priority | Active online workload receives its `max` budget; non-peak workload receives normal preferred budget | live moderation -> `master0`; office agent -> `worker1` | 2/2 `Applied`; live moderation message reports `tier="max"` |
| Online/offline borrowing | Online decode-heavy and offline prefill-heavy workloads are placed on complementary nodes | online chat -> `worker0`; batch embedding -> `worker1` | 2/2 `Applied` |
| SLO-risk avoidance | Latency-sensitive workload avoids the configured high-risk node `master1` | risk-sensitive chat -> `master0` | 1/1 `Applied` |

## Evidence

Each scenario directory contains:

- `SUMMARY.md`: concise Pod, allocation, and node-profile state
- `pods.yaml` and `pods-wide.txt`: Kubernetes placement evidence
- `allocations.yaml`: desired and observed AI resource allocations
- `nodeprofiles.yaml`: node capacity, availability, pressure, and allocation ledger
- `events.txt`: Kubernetes scheduling and startup events
- `astra-scheduler.log`: scheduler decisions
- `astra-agent-master0.log`: local agent reconciliation log

Only the master0 systemd agent log was captured directly. Remote agents are evidenced by their node-bound allocations reaching `Applied` and updating the corresponding node profiles.

## Issues Found During Evaluation

The deployment exercise exposed and fixed two controller correctness issues:

1. `AINodeResourceProfile.status` updates could fail indefinitely on a stale `resourceVersion`. The agent now retries conflicts using the latest object.
2. Agent-written allocation status updates retriggered reconciliation and caused `Applied`/`Applying` oscillation. Watches now react to generation changes, and already-applied identical plans are skipped.
3. A locally rejected plan was previously passed to the backend and then marked `Applied`. Failed plans now terminate as `Failed` without backend application.

## Interpretation

This run validates the current prototype's functional chain:

```text
AIWorkloadProfile
  -> Astra scheduler Filter/Score
  -> Kubernetes Pod binding
  -> AIResourceAllocation
  -> node-local Astra agent plan
  -> fake backend application
  -> allocation and node-profile status update
```

The run demonstrates functional feasibility, not comparative scheduling superiority. Performance, utilization, SLO violation rate, fairness, and baseline comparisons require the next evaluation phase with repeated workloads and quantitative measurements.
