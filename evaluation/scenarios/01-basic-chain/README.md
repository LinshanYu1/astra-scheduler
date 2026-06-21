# 01 Basic Chain

## Goal

Validate the minimal Astra scheduling chain:

```text
Pod -> astra-scheduler -> AIResourceAllocation -> astra-agent -> Applied -> AINodeResourceProfile status
```

## What It Tests

- The workload pod uses `schedulerName: astra-scheduler`.
- The scheduler reads `AIWorkloadProfile`.
- The scheduler creates `AIResourceAllocation`.
- The node agent observes the allocation and marks it `Applied`.
- The node resource profile reflects the applied allocation.

## Expected Result

- `basic-chain-chat` becomes `Running`.
- One `AIResourceAllocation` is created for the pod.
- The allocation phase becomes `Applied`.
- The chosen node's `AINodeResourceProfile.status.allocations` includes this allocation.

