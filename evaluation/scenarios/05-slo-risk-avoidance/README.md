# 05 SLO-Risk Avoidance

## What Is SLO Risk?

SLO means **Service Level Objective**. In AI serving, common SLOs include:

- TTFT P95, time to first token at the 95th percentile;
- latency P99;
- timeout rate;
- queue depth;
- KV-cache pressure.

SLO risk is a coarse signal that the current node is close to violating one or
more service objectives. In this fake evaluation, the signal comes from
`AINodeResourceProfile.status.runtime.pressure.sloRisk`.

## Goal

Validate that Astra avoids nodes that have enough static resources but high
runtime risk.

## Expected Result

`risk-sensitive-chat` should avoid `master1`, because the base fake cluster marks
`master1` as:

```yaml
sloRisk: high
queueDepth: 12
```

