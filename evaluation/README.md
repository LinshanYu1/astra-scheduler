# Astra Scheduler Evaluation

This directory contains small, repeatable evaluation scenarios for the current
Astra Scheduler prototype.

The goal is not to benchmark real GPUs yet. The current version evaluates
whether the scheduler and agent can make explainable decisions from AI workload
characteristics:

- required / preferred / max resource budgets
- demand shape: `decode_heavy`, `prefill_heavy`, `kv_heavy`, `mixed`
- business time windows
- priority
- online/offline co-scheduling
- node runtime pressure

## Directory Layout

```text
evaluation/
  base/
    fake-cluster.yaml
  scenarios/
    01-basic-chain/
    02-complementary-colocation/
    03-peak-reclaim/
    04-time-window-colocation/
    05-slo-risk-avoidance/
    01-resource-shape-placement/
    02-time-window-priority/
    03-online-offline-borrowing/
    04-slo-risk-avoidance/
  scripts/
    run-scenario.sh
    cleanup-scenario.sh
```

## How to Run

Make sure the Astra CRDs, scheduler, and agents are already deployed.

Apply the fake node resource profiles:

```bash
kubectl apply -f evaluation/base/fake-cluster.yaml
```

Run a scenario:

```bash
./evaluation/scripts/run-scenario.sh 01-resource-shape-placement
```

Check results:

```bash
kubectl get pods -n astra-eval -o wide
kubectl get aiworkloadprofiles -n astra-eval
kubectl get airesourceallocations -n astra-eval -o wide
kubectl get ainoderesourceprofiles -n astra-scheduler-system
```

Clean a scenario:

```bash
./evaluation/scripts/cleanup-scenario.sh 01-resource-shape-placement
```

## What We Evaluate

The current recommended scenario set is:

| Scenario | Purpose |
| --- | --- |
| `01-basic-chain` | Validate the basic scheduler to allocation to agent apply chain. |
| `02-complementary-colocation` | Validate that workloads with complementary demand shapes can be safely co-located. |
| `03-peak-reclaim` | Validate high-priority peak reclaim and replacement-pod based rescheduling. |
| `04-time-window-colocation` | Validate that same-priority workloads with non-overlapping peak windows can share a node. |
| `05-slo-risk-avoidance` | Validate that latency-sensitive workloads avoid nodes with high runtime SLO risk. |

### 1. Placement Correctness

Whether workloads are placed on nodes whose fake resource profiles match their
dominant resource needs.

Examples:

- `decode_heavy` should prefer nodes with more decode token headroom.
- `prefill_heavy` should prefer nodes with more prefill token headroom.
- `kv_heavy` should prefer nodes with more KV cache headroom.

### 2. Time-Window Behavior

Whether workloads in active peak windows receive `max` or higher local budgets
from the node agent, while non-peak or lower-priority workloads stay closer to
`required` or `preferred`.

### 3. Online/Offline Co-Scheduling

Whether offline workloads can use spare capacity while online workloads keep
their required or peak budgets.

### 4. SLO-Risk Awareness

Whether high-risk nodes are less attractive for latency-sensitive online
workloads.

SLO means **Service Level Objective**. In this project, SLO risk is not a
contract itself. It is a runtime signal that a node or serving stack is close to
violating objectives such as TTFT P95, latency P99, queue depth, timeout rate,
or KV-cache pressure. The fake evaluation exposes this through
`AINodeResourceProfile.status.runtime.pressure.sloRisk`.

## Baselines to Compare Later

These scenarios are designed so we can later compare Astra with:

- Kubernetes default scheduler
- static node placement
- resource-only scheduler
- Astra Scheduler

The first two baselines can be evaluated using the same workloads by removing
`spec.schedulerName: astra-scheduler` or replacing it with fixed node
selectors.

## Run And Record All Scenarios

From a host with cluster access:

```bash
./evaluation/scripts/run-all-and-record.sh
```

Run a single scenario:

```bash
./evaluation/scripts/run-all-and-record.sh 01-resource-shape-placement
```

Results are stored under `evaluation/results/<timestamp>/`, and `evaluation/results/latest` points to the most recent run.
