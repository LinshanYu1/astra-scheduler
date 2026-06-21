# Scenario 01: Resource Shape Placement

This scenario checks whether Astra places different AI workload shapes on nodes
with matching resource headroom.

Workloads:

- `eval-decode-chat`: online, `decode_heavy`
- `eval-prefill-embedding`: offline, `prefill_heavy`
- `eval-kv-agent`: online, `kv_heavy`

Expected behavior:

- decode-heavy workload should prefer the node with stronger decode throughput.
- prefill-heavy workload should prefer the node with stronger prefill throughput.
- kv-heavy workload should prefer the node with more KV cache headroom.

Run:

```bash
./evaluation/scripts/run-scenario.sh 01-resource-shape-placement
kubectl get pods -n astra-eval -o wide
kubectl get airesourceallocations -n astra-eval -o wide
```
