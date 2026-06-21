# 03 Peak Reclaim

## Goal

Validate the two-level scheduling loop:

```text
low-priority workload occupies a node
high-priority workload enters peak window and needs max budget
agent creates a replacement pod for the low-priority workload
replacement pod re-enters astra-scheduler
old allocation is released after the replacement is running
```

## Why This Matters

This scenario demonstrates consistency between central scheduling and node-local
scheduling. The agent does not mutate an already-bound pod back to Pending.
Instead, it creates a replacement pod without `spec.nodeName`, so Kubernetes
places the replacement pod back into the normal scheduling path.

## Expected Result

- `peak-live-master0` runs on `master0` and is assigned `max` resources.
- `peak-low-offline-source` is first present on `master0`.
- The old low-priority allocation becomes `Migrating`, then `Released`.
- A replacement pod named like `peak-low-offline-source-astra-*` is created.
- The replacement pod is scheduled by `astra-scheduler` to another node.

