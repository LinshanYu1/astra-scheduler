# Scenario 03: Online and Offline Borrowing

This scenario checks whether low-priority offline work can coexist with online
workloads and use spare capacity through the agent policy.

Workloads:

- `eval-chat-service`: high-priority online workload.
- `eval-batch-embedding`: low-priority offline workload, borrowable,
  throttleable, pauseable, and evictable.

Expected behavior:

- Online workload keeps at least its required/preferred budget.
- Offline workload can be assigned capacity if enough headroom remains.
- If resources become tight in future tests, the offline workload is the first
  candidate for throttle/pause/evict.

Run:

```bash
./evaluation/scripts/run-scenario.sh 03-online-offline-borrowing
kubectl get pods -n astra-eval -o wide
kubectl get airesourceallocations -n astra-eval -o yaml
```
