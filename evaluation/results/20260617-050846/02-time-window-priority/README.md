# Scenario 02: Time Window and Priority

This scenario checks whether the node-side agent can assign higher local budgets
to workloads in active peak windows.

Workloads:

- `eval-live-moderation`: high priority online workload, peak all day in this
  test, so the agent should try to apply `max`.
- `eval-office-agent`: medium priority online workload, normal preferred budget.

Expected behavior:

- The scheduler places both workloads only if required/preferred/max envelopes are feasible.
- The agent should apply `max` or near-max resources to the active-window high-priority workload.
- The allocation status message should mention active time window and max budget.
 
Run:

```bash
./evaluation/scripts/run-scenario.sh 02-time-window-priority
kubectl get airesourceallocations -n astra-eval -o yaml
```
