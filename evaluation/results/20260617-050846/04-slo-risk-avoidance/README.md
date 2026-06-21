# Scenario 04: SLO-Risk Avoidance

This scenario checks whether latency-sensitive workloads avoid nodes whose fake
runtime pressure reports higher SLO risk.

The base fake cluster marks `master1` with:

```yaml
sloRisk: high
queueDepth: 12
```

Workload:

- `eval-risk-sensitive-chat`: high-priority online workload.

Expected behavior:

- The workload should prefer a lower-risk node if resource capacity is otherwise
  sufficient.

Run:

```bash
./evaluation/scripts/run-scenario.sh 04-slo-risk-avoidance
kubectl get pods -n astra-eval -o wide
kubectl get airesourceallocations -n astra-eval -o yaml
```
