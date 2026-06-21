# 04 Time-Window Colocation

## Goal

Validate that same-priority online workloads can share a node when their peak
windows do not overlap.

## Scheduling Semantics

If two online workloads have the same priority:

- overlapping windows require `A.max + B.max <= node capacity`;
- non-overlapping windows require both:
  - `A.max + B.required <= node capacity`;
  - `B.max + A.required <= node capacity`.

## Expected Result

Two high-priority online workloads with non-overlapping peak windows should be
allowed to colocate on a node that can satisfy the envelope above.

This scenario pins both workloads to `worker0` with `nodeSelector` so the test
isolates the time-window hard constraint. If the envelope is invalid, the second
pod should stay Pending. If the envelope is valid, both pods should run on
`worker0`.
