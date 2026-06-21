# 02 Complementary Colocation

## Goal

Validate that Astra can place complementary AI workloads on the same strong node
when their dominant resource pressures differ.

## Workloads

- `colo-decode-chat`: online, decode-heavy.
- `colo-prefill-embedding`: offline, prefill-heavy.
- `colo-kv-agent`: online, KV-cache-heavy.

## What It Tests

The scenario checks whether the scheduler can use resource shape and headroom,
instead of only spreading pods uniformly. The desired behavior is to colocate
workloads that can safely share a node because their bottleneck resources differ.

## Expected Result

- The decode-heavy and KV-heavy online workloads should prefer the node with high
  decode and KV headroom.
- The prefill-heavy workload should prefer the node with high prefill headroom.
- At least two workloads should be placed on the same node when the node has
  enough headroom.

