# Astra Agent Fake Deployment

This directory contains the first runnable node-side deployment for
`astra-agent`.

The current deployment uses the `fake` backend. It does not touch real GPU
hardware. Instead, each agent pod reads node capacity from a ConfigMap and uses
that data as the node-local resource snapshot. This lets us verify the whole
scheduler-agent chain without a GPU machine.

## Components

- `daemonset.yaml`: runs one `astra-agent` pod on every Kubernetes node.
- `fake-node-resources.yaml`: stores fake AI resources per node name.
- `kustomization.yaml`: includes both resources.

When included from `config/default`, kustomize adds the project name prefix.
That is why the DaemonSet points to:

```text
astra-scheduler-fake-node-resources
```

## Fake Node Resource Format

Each ConfigMap key can be one of:

```text
<node-name>.yaml
<node-name>.yml
<node-name>
resources.yaml
resources.yml
```

The node-specific key has priority. `resources.yaml` can be used as a fallback
for all nodes.

Example:

```yaml
data:
  worker-a.yaml: |
    allocatable:
      gpuCount: 4
      gpuMemoryGiB: 320
      kvCacheGiB: 128
      prefillTokensPerSecond: 6000
      decodeTokensPerSecond: 9000
      totalTokensPerSecond: 12000
    runtime:
      pressure:
        sloRisk: low
        queueDepth: 2
```

For a local fake demo, edit the keys so they match your real Kubernetes node
names from:

```bash
kubectl get nodes
```

## Deploy

From the project root:

```bash
kubectl apply -k config/default
```

Check the agent:

```bash
kubectl get pods -n astra-scheduler-system -l app.kubernetes.io/component=astra-agent -o wide
kubectl logs -n astra-scheduler-system -l app.kubernetes.io/component=astra-agent --tail=100
```

## Verify Fake Resource Sync

The agent creates or updates one `AINodeResourceProfile` per node and writes
the observed fake resources into status.

```bash
kubectl get ainoderesourceprofiles -n astra-scheduler-system
kubectl describe ainoderesourceprofile -n astra-scheduler-system <node-name>
```

To simulate a resource change, edit the ConfigMap and watch the profile status:

```bash
kubectl edit configmap -n astra-scheduler-system astra-scheduler-fake-node-resources
kubectl get ainoderesourceprofile -n astra-scheduler-system <node-name> -o yaml
```

## End-to-End Check

After the central scheduler creates an `AIResourceAllocation`, the agent should:

1. list allocations assigned to its node;
2. build local plans from priority, time window, and available fake resources;
3. apply plans through the fake backend;
4. update `AIResourceAllocation.status`;
5. refresh `AINodeResourceProfile.status`.

Useful commands:

```bash
kubectl get airesourceallocations -A
kubectl describe airesourceallocation -n <namespace> <name>
kubectl get ainoderesourceprofiles -n astra-scheduler-system -o yaml
```

