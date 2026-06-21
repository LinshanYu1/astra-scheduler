# Scenario Validation Results

Date: 2026-06-21

This run validates the current fake end-to-end Astra Scheduler prototype on the
four-node local Kubernetes cluster.

The raw rerun output for scenarios 03 and 04 was captured on `master0` at:

```text
/home/master0/astra-eval-results/20260621-035037-rerun-03-04
```

## SLO Risk

SLO means **Service Level Objective**.

In this project, SLO risk means the runtime risk that an online AI workload is
close to violating its service objectives. Typical signals include:

- TTFT P95, time to first token at the 95th percentile;
- latency P99;
- queue depth;
- timeout rate;
- KV-cache pressure.

The current fake evaluation represents this signal with:

```yaml
status:
  runtime:
    pressure:
      sloRisk: high
      queueDepth: 12
```

## Results

| Scenario | Expected Behavior | Observed Result | Status |
| --- | --- | --- | --- |
| `01-basic-chain` | Pod is scheduled by `astra-scheduler`; allocation is created and applied by the agent. | `basic-chain-chat` ran on `worker0`; allocation phase became `Applied`. | PASS |
| `02-complementary-colocation` | Complementary workload shapes should be placed according to resource fit and co-location safety. | `colo-decode-chat` and `colo-kv-agent` ran on `worker0`; `colo-prefill-embedding` ran on `worker1`; allocations were `Applied`. | PASS |
| `03-peak-reclaim` | High-priority peak workload should reclaim `master0`; low-priority workload should be replaced and rescheduled. | `peak-live-master0` ran on `master0` with max budget; old low-priority allocation became `Released`; replacement pod `peak-low-offline-source-astra-*` ran on `worker1`. | PASS |
| `04-time-window-colocation` | Same-priority online workloads with non-overlapping peak windows can share one node. | `window-office-agent` and `window-live-moderation` both ran on `worker0`; allocations were `Applied`. | PASS |
| `05-slo-risk-avoidance` | Latency-sensitive workload should avoid a node with high runtime SLO risk. | `risk-sensitive-chat` avoided `master1`, whose fake profile has `sloRisk: high` and `queueDepth: 12`. | PASS |

## Interpretation

The scenarios show that the current prototype can exercise the intended
two-level scheduling loop:

1. The central scheduler performs hard filtering and soft scoring based on
   workload profile, resource fit, time windows, demand shape, and runtime risk.
2. The scheduler creates `AIResourceAllocation` objects as explicit scheduling
   decisions.
3. The node-side agent turns allocations into local plans and applies them
   through the fake backend.
4. When a peak workload needs to reclaim resources, the agent can create a
   replacement pod for a lower-priority workload, return that pod to the normal
   Kubernetes scheduling path, and release the old allocation after the
   replacement is running.

This is still a fake-resource evaluation. It validates scheduling behavior and
control-plane consistency, not real GPU isolation or real KV-cache enforcement.
