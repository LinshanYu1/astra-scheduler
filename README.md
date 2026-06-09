# astra-scheduler

## Description

`astra-scheduler` is an experimental Kubernetes-native scheduler for mixed AI
workloads. The project explores how an upper-layer scheduler can use workload
characteristics, time windows, SLO risk, and AI-specific resource pressure to
co-schedule latency-sensitive online inference workloads and delay-tolerant
offline workloads.

The first version focuses on four resource dimensions:

- GPU device capacity
- MIG or GPU partition capacity
- KV cache budget
- token throughput budget

The goal is not to replace existing GPU device plugins or Kubernetes resource
mechanisms. Instead, `astra-scheduler` treats those lower-level capabilities as
execution backends and studies the scheduling policy layer above them.

## Motivation

Kubernetes and the open-source ecosystem already provide many important building
blocks for AI infrastructure:

- GPU devices can be exposed to Pods through device plugins and extended
  resources.
- NVIDIA GPU Operator and device plugins support MIG, GPU sharing, time-slicing,
  and related GPU management features.
- Kubernetes Dynamic Resource Allocation is evolving toward a more structured
  way to request and allocate devices.
- Schedulers such as KAI Scheduler, Volcano, and Koordinator provide GPU-aware,
  queue-aware, batch-oriented, or fine-grained resource scheduling capabilities.
- LLM serving runtimes such as vLLM, SGLang, TGI, TensorRT-LLM, and related
  systems optimize batching, KV cache management, prefix caching, and token
  throughput inside the serving layer.

However, these capabilities are often handled at different layers. Kubernetes
usually sees coarse resources such as CPU, memory, and GPU count. LLM runtimes
understand KV cache, prefill/decode behavior, queue depth, and token throughput,
but these signals are not usually first-class scheduling inputs at the
Kubernetes workload level.

`astra-scheduler` focuses on this gap:

```text
workload characteristics
  + business time windows
  + SLO risk
  + online/offline workload type
  + GPU/MIG/KV/token budgets
  -> scheduling and allocation decisions
```

The first prototype intentionally separates policy from enforcement. The
scheduler reads Kubernetes CRDs and produces allocation decisions. A node-side
agent applies those decisions through a pluggable backend. In experiments, the
agent can use a fake backend backed by local configuration. In future versions,
the same backend interface can be implemented by integrations with NVIDIA
tooling, DCGM, device plugins, Dynamic Resource Allocation, CRI, containerd, or
other runtime-level mechanisms.

## Scope

The current project should be understood as a scheduling-policy prototype, not a
new GPU driver or a replacement for Kubernetes device plugins.

In scope for the first version:

- CRDs for AI workload profiles, node resource profiles, and resource
  allocations.
- A custom scheduler that reasons about workload type, priority, SLO risk, time
  windows, GPU capacity, MIG capacity, KV cache budget, and token throughput.
- A node-side agent with a pluggable resource backend.
- A fake backend for paper/demo evaluation without requiring real GPUs.
- A future path for real backends that integrate with GPU and runtime systems.

Out of scope for the first version:

- modifying kubelet;
- modifying CRI or container runtime internals;
- dynamically reconfiguring real MIG partitions;
- replacing vLLM/SGLang/TGI runtime schedulers;
- training a production-grade optimizer.

The long-term research question is:

```text
How can a Kubernetes-native scheduler use AI workload characteristics,
temporal demand patterns, and SLO risk signals to improve GPU utilization
while protecting latency-sensitive online AI services?
```

## Resource Model

The first version models four AI scheduling resources.

### GPU

GPU is the physical accelerator capacity. The scheduler can use GPU count, GPU
type, GPU memory, and available compute capacity to decide whether a workload can
fit on a node.

### MIG

MIG represents partitioned GPU capacity, such as fixed GPU slices exposed as
profiles. The prototype treats MIG profiles as schedulable capacity reported by
the node agent. The fake backend can simulate allocation and release of MIG
profiles, while a future real backend can integrate with NVIDIA tooling or
Kubernetes device mechanisms.

### KV Cache

KV cache is an LLM inference runtime resource backed by GPU memory. It grows
with context length, concurrent requests, generated tokens, model size, and
serving configuration. The scheduler models KV cache as a budget so that long
context or high-concurrency workloads do not overload a node even when the raw
GPU count appears sufficient.

### Token Throughput

Token throughput represents LLM serving capacity, such as input tokens/sec,
output tokens/sec, prefill capacity, or decode capacity. It is closer to user
visible serving performance than raw GPU utilization. The scheduler can use this
budget to protect online workloads and allow offline workloads to borrow spare
capacity when SLO risk is low.

## Architecture

The first version is designed around two processes:

```text
astra-scheduler
  -> watches workload profiles and node resource profiles
  -> computes scheduling decisions
  -> writes AIResourceAllocation objects

astra-agent
  -> runs on each node
  -> reports AINodeResourceProfile status
  -> watches allocations assigned to the node
  -> applies or releases resources through a backend plugin
```

The agent backend is intentionally pluggable:

```text
fake backend
  -> used for local testing and paper evaluation

real backend
  -> future integration point for NVIDIA/DCGM/device-plugin/DRA/CRI/runtime
```



## Pre-design Notes: Lessons from Edge-Cloud Scheduling

This project is also inspired by an earlier edge-cloud scheduling system. That
system handled heterogeneous edge nodes, limited bandwidth, local disks, task
instances, and business-specific deployment policies. Although the target
resource is different, several design ideas map naturally to AI workload
scheduling.

### Two-level Scheduling

The edge-cloud system separated scheduling into two levels:

```text
control plane / operator
  -> understands task metadata and desired placement
  -> produces a deployment policy

node-side tasklet
  -> observes local node resources
  -> reconciles the desired policy with real local state
  -> applies concrete resource operations
```

`astra-scheduler` should follow the same pattern:

```text
astra-scheduler
  -> makes cluster-level AI scheduling decisions
  -> selects nodes and resource budgets
  -> writes AIResourceAllocation

astra-agent
  -> runs on each node
  -> reports local GPU/MIG/KV/token capacity
  -> reconciles allocations through a backend plugin
```

This avoids putting all scheduling logic into a single global component. The
cluster scheduler makes policy decisions, while the node agent handles local
resource details and eventual consistency.

### CRD Mapping

The earlier system used separate objects for task metadata, node configuration,
node status, and generated task plans. The same separation is useful here.

| Edge-cloud scheduler concept | Astra concept | Purpose |
| --- | --- | --- |
| Task metadata / task config | `AIWorkloadProfile` | Describes workload type, priority, SLO, time window, required/preferred resources |
| Node config / node info | `AINodeResourceProfile` | Describes observed node-side AI resources and agent status |
| Task plan / deployment policy | `AIResourceAllocation` | Records the scheduler's allocation decision for a workload on a node |
| Tasklet | `astra-agent` | Applies node-local allocation and reports actual state |
| Policy engine | scheduler policy engine | Filters and scores candidate nodes, then computes resource budgets |

This keeps the API Kubernetes-native and makes each object easy to reason about.

### Required and Preferred Resource Model

The edge-cloud policy engine distinguished between required and preferred
resources. Required resources determine whether a task can run at all; preferred
resources determine how well it can run.

For AI workloads, the same idea is important:

```text
required GPU / MIG / KV / token budget
  -> hard feasibility check

preferred GPU / MIG / KV / token budget
  -> scoring and optimization target
```

For example, an online LLM service may require at least one GPU and a minimum KV
cache budget, but it may prefer more decode throughput during peak time. An
offline embedding job may require only a small minimum budget and opportunistically
consume more capacity when the cluster is idle.

### Resource-specific Policy Engines

The old system split policy logic by resource type, such as disk policy and line
or bandwidth policy. `astra-scheduler` can use the same structure instead of
putting all decisions into one large scheduler function.

Possible sub-policies:

- `GPUPolicy`: GPU count, GPU type, GPU memory, compute capacity.
- `MIGPolicy`: fixed GPU partition profiles and slice availability.
- `KVCachePolicy`: KV cache budget, context length pressure, concurrency risk.
- `TokenThroughputPolicy`: prefill/decode/token throughput capacity.
- `TimeWindowPolicy`: peak/off-peak workload importance.
- `SLORiskPolicy`: TTFT, p95/p99 latency, queue depth, timeout risk.
- `BorrowingPolicy`: whether low-priority offline work can borrow spare capacity.

The scheduler can compose these policy outputs into one final node score and
allocation decision.

### Plan and Reconcile Loop

The edge-cloud tasklet did not assume that a generated plan was immediately true.
It compared desired state with actual local state and reconciled the difference.
This is also the right model for AI resources.

```text
AIResourceAllocation.spec
  -> desired allocation generated by astra-scheduler

AIResourceAllocation.status
  -> actual allocation reported by astra-agent
```

The agent should continuously reconcile desired allocation against backend state.
For the first version, the backend can be fake and backed by local files or
in-memory state. Later, the same reconcile loop can call real GPU/runtime APIs.

### Stable Resource Identity

The edge-cloud system used stable disk identity instead of relying on unstable
array order. AI resource scheduling needs the same discipline.

For future real backends, resource identity should be based on stable names such
as:

- GPU UUID
- MIG device UUID
- node name
- runtime instance name
- model replica identity

This is useful for debugging, recovery after restart, and avoiding accidental
allocation drift.

### Plugin-oriented Agent Backend

The earlier system had separate managers for different local operations. Astra
should keep this idea by making the agent backend replaceable.

```text
ResourceBackend interface
  -> FakeBackend for demo and evaluation
  -> NvidiaBackend for future GPU/MIG integration
  -> RuntimeBackend for future vLLM/SGLang/token/KV integration
  -> CRIBackend for deeper container runtime integration
```

This makes the first prototype feasible without real GPUs while preserving a
clear path toward production-grade integration.

### What to Borrow and What Not to Borrow

Ideas worth borrowing:

- two-level scheduler plus node-side agent;
- desired state and actual state separation;
- required/preferred resource policy;
- resource-specific sub-policy engines;
- stable local resource identity;
- reconcile-based node agent;
- pluggable local backend;
- status feedback from node agent to scheduler.

Ideas not to copy directly:

- too much custom node communication if Kubernetes watch/status updates are
  sufficient;
- edge-specific disk and network assumptions;
- imperative local orchestration that can be represented as Kubernetes desired
  state;
- tightly coupling policy logic with backend execution logic.

### Design Implication for Version 1

The first implementation should prioritize a clean scheduling loop over deep GPU
integration:

```text
1. Define AIWorkloadProfile, AINodeResourceProfile, and AIResourceAllocation.
2. Implement a scheduler policy engine using required/preferred resources.
3. Implement astra-agent with a fake backend.
4. Let the fake backend simulate GPU/MIG/KV/token allocation.
5. Evaluate time-aware and SLO-aware scheduling decisions with synthetic or
   replayed workload traces.
```

This gives the project a working research prototype while leaving the lower-level
GPU/runtime integration as a clear future extension.

## Current Implementation Status

This repository is currently in the first prototype stage. The goal of this
stage is to make the scheduling architecture executable and easy to extend,
rather than to integrate with real GPU hardware immediately.

### Implemented

The current version has implemented the following pieces.

#### Kubernetes API Model

Three CRDs define the core scheduling objects:

- `AIWorkloadProfile`: describes one AI workload's scheduling intent, including
  workload type, priority, demand shape, SLO, time windows, required resources,
  preferred resources, and local adjustment policy.
- `AINodeResourceProfile`: describes AI resource capacity and observed status on
  one node, including GPU, runtime capacity, KV cache budget, token throughput,
  backend type, runtime support, and node-side capabilities.
- `AIResourceAllocation`: records one scheduler decision for one workload on one
  node. The scheduler writes the desired allocation, and the agent later reports
  the actual applied status.

#### Custom Scheduler Binary

The project now contains a standalone scheduler binary under `cmd/scheduler`.
It registers the `AstraScheduler` scheduler framework plugin and can be used by
Pods through `spec.schedulerName: astra-scheduler`.

The scheduler plugin currently implements:

- `PreFilter`: resolves and caches the `AIWorkloadProfile` for the Pod.
- `Filter`: performs hard feasibility checks.
- `PreScore`: prepares per-cycle scheduling state.
- `Score`: calculates preferred resource coverage and records candidate node
  details into `CycleState`.
- `NormalizeScore`: compares candidate nodes in the best preferred-resource
  tier and produces final scores.
- `Reserve` / `Unreserve`: creates or cleans up `AIResourceAllocation` objects
  as scheduler-side reservation records.

#### Hard Constraint Filtering

The scheduler filter stage now checks hard constraints before scoring:

- the node must have a usable `AINodeResourceProfile`;
- required GPU, GPU memory, KV cache, and token throughput must fit;
- workloads that require KV cache must run on nodes whose runtime supports KV
  cache;
- workloads that require prefill/decode/total token budget must run on nodes
  whose runtime supports token throughput control;
- workload policies that require borrowing, reclaim, throttling, pause/resume,
  or eviction must match node-side capabilities.

This keeps non-negotiable constraints in the central scheduler rather than
leaving them to the node agent after placement.

#### Preferred-resource Scoring

The current scoring policy uses a clear two-stage idea:

```text
required resources
  -> hard filter

preferred resource coverage
  -> primary score tier

demand-shape headroom and runtime risk
  -> same-tier refinement
```

`CycleState` records the best preferred-resource tier during `Score`. Nodes that
match the same number of preferred resource dimensions share the same intermediate
score. A node that matches more preferred dimensions replaces the current best
tier. Nodes below the best tier are kept but are scored below the best tier during
normalization.

Within the best tier, the policy compares:

- the key resource implied by workload demand shape, such as KV cache for
  `kv_heavy`, decode throughput for `decode_heavy`, and prefill throughput for
  `prefill_heavy`;
- key-resource headroom;
- other resource headroom;
- runtime SLO risk penalties.

This keeps the first version explainable while leaving room for a learned scoring
policy later.

#### Node-side Agent Controller

The project now contains an `astra-agent` binary under `cmd/agent`. The agent is
intended to run as a DaemonSet. Each agent instance receives its node name through
the Kubernetes downward API, for example:

```yaml
env:
  - name: NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
```

The agent is implemented as a Kubernetes controller, not as a polling loop. It
watches `AIResourceAllocation` objects, filters events for the current node, and
reconciles node-local allocation state.

#### Agent Internal Architecture

The node-side agent is split into extensible layers:

```text
agent controller
  -> watches AIResourceAllocation

local scheduler / arbiter
  -> produces node-local plans

resource managers
  -> GPU manager
  -> KV cache manager
  -> token throughput manager

constraint engine
  -> cross-resource and action-capability checks

backend driver
  -> fake backend in the current version
```

The first agent version supports:

- listing all desired allocations assigned to the current node;
- ordering local plans by workload priority;
- validating GPU count and GPU memory;
- validating KV cache budget;
- validating prefill/decode/total token throughput budget;
- checking cross-resource constraints such as token throughput requiring GPU
  capacity;
- checking runtime action capability for throttle, pause, and evict actions;
- applying plans through a fake backend;
- updating `AIResourceAllocation.status`;
- refreshing `AINodeResourceProfile.status`.

The fake backend does not operate real GPU devices. It simulates accounting and
status updates so that the scheduler-agent control loop can be tested without a
GPU cluster.

#### Tests

The current repository includes focused unit tests for:

- scheduler scoring behavior;
- preferred-resource coverage priority;
- key-resource headroom comparison;
- hard constraint filtering;
- allocation resource selection;
- node-local agent plan ordering;
- fallback local actions when desired allocation exceeds local capacity.

The current code path is verified with:

```sh
go test ./...
```

### Not Implemented Yet

The following pieces are intentionally left for later versions:

- real GPU/MIG enforcement;
- real KV cache and token budget control through vLLM, SGLang, TGI, or other
  runtimes;
- Dynamic Resource Allocation integration;
- kubelet or CRI integration;
- request-level routing;
- production-grade time-window policy;
- production-grade SLO risk estimation;
- learned scheduling policy;
- full scheduler deployment manifests;
- full agent DaemonSet and RBAC manifests;
- end-to-end evaluation with replayed or synthetic AI workload traces.

## Roadmap

The project roadmap is split into near-term engineering tasks and longer-term
research goals.

### Version 1: Working Kubernetes Prototype

The first milestone is to make the core scheduler-agent loop run inside a local
Kubernetes cluster.

Planned work:

- add scheduler deployment manifests and RBAC;
- add agent DaemonSet manifests and RBAC;
- create sample Pods that use `spec.schedulerName: astra-scheduler`;
- create sample `AIWorkloadProfile`, `AINodeResourceProfile`, and
  `AIResourceAllocation` objects;
- verify that the scheduler writes allocations and the agent applies them with
  the fake backend;
- record simple scheduling results for online and offline workload examples.

### Version 1.5: Workload-mix-aware Scoring

The current scoring logic already considers the current workload's preferred
resources. The next scoring improvement is to model node-level workload mix.

The scheduler should not only ask:

```text
Does this node have enough preferred resources for the current workload?
```

It should also ask:

```text
Will placing this workload on the node make the node's workload mix healthier
or more imbalanced?
```

For example, placing many `decode_heavy` workloads on the same node may create a
decode bottleneck even if the node still has apparent spare capacity. A
`prefill_heavy` or `kv_heavy` workload may be a better complement on that node.

Planned work:

- compute demand-shape distribution from `AINodeResourceProfile.status.allocations`;
- add a workload-mix balance score;
- penalize concentration of the same bottleneck type;
- reward complementary placement when a workload uses less-contended resources;
- keep this score secondary to required feasibility and preferred-resource
  coverage.

This directly reflects the project's research idea: AI workloads should be
co-scheduled based on task characteristics, not only raw GPU count.

### Version 2: Realistic Node-local Arbitration

The second milestone is to make the agent behave more like a true node-local
scheduler.

Planned work:

- convert `BuildPlans` from validation-oriented logic into allocation-oriented
  logic;
- allocate resources from high priority to low priority;
- guarantee required resources first;
- assign preferred resources when possible;
- allow low-priority workloads to borrow idle capacity;
- throttle or pause low-priority offline workloads when online SLO risk rises;
- support resume when pressure drops;
- add clear status reasons for every local action.

This stage makes the two-level scheduling design meaningful: the central
scheduler chooses the node, and the agent performs local arbitration based on
real-time node pressure.

### Version 3: Runtime and GPU Backend Integration

The third milestone is to replace the fake backend with real or semi-real
backends.

Possible backend integrations:

- NVIDIA GPU and MIG state observation;
- DCGM metrics;
- Kubernetes device plugin state;
- DRA `ResourceClaim` / `ResourceSlice` concepts;
- vLLM or SGLang runtime APIs for concurrency, queue, KV cache, and token budget;
- container runtime or CRI integration for deeper enforcement.

The first real backend does not need to support every operation. A useful first
step is read-only observation of GPU/runtime pressure, followed by controlled
throttling of low-priority workloads.

### Version 4: Research Evaluation

The long-term goal is to evaluate whether workload-characteristic-aware
scheduling improves AI cluster efficiency.

Evaluation should compare:

- Kubernetes default scheduling;
- round-robin routing;
- least-queue routing;
- static priority scheduling;
- Astra's time-aware, SLO-aware, workload-mix-aware scheduling.

Potential metrics:

- GPU utilization;
- high-priority p95/p99 latency;
- TTFT and TPOT;
- SLO violation rate;
- low-priority job completion time;
- token throughput;
- preemption/throttle/pause count;
- scheduling overhead;
- fairness and resource waste.

### Long-term Research Direction

The long-term research question is whether Kubernetes-native AI scheduling can
use workload characteristics, temporal patterns, SLO risk, and runtime pressure
to co-schedule online and offline AI workloads better than coarse GPU-count
scheduling.

Future research ideas include:

- learned workload profiling;
- learned resource demand prediction;
- reinforcement-learning or bandit-based local arbitration;
- workload-mix-aware scheduling policies;
- sampling-based routing inspired by Sparrow;
- joint scheduling of prefill, decode, KV cache, and GPU memory;
- integration with DRA or future Kubernetes AI resource APIs.

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/astra-scheduler:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/astra-scheduler:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/astra-scheduler:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/astra-scheduler/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
