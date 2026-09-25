# Jevlet

**Jevlet is a Jev-powered System One control loop for Kubernetes.**

It is built for the class of Kubernetes failures that are often obvious only after combining several pieces of cluster state — scheduler constraints, missing objects, broken selectors, network signals, stale release dependencies, rollout state — rather than simply reading application logs.

> **Jev decides. Jevlet authorizes and executes.**

Jevlet never asks Jev to generate shell commands, YAML, or arbitrary Kubernetes API calls. Jev can only return values from a closed decision schema. Ordinary Go code owns authorization, policy, and actuation.

## Why this exists

A lot of Kubernetes incidents do **not** begin with a useful application log.

Sometimes the container never starts.

Sometimes there is no Pod to inspect.

Sometimes all Pods are `Running`, but traffic is crawling.

Sometimes Helm reports `STATUS: deployed` while runtime dependencies are stale.

Sometimes the scheduler is doing exactly what you asked — and that is the problem.

The architecture Jevlet is exploring is:

```text
Kubernetes state
Events
Scheduler state
Prometheus
optional external health
       |
       v
   StateBuilder
       |
       v
       Jev
 Choice / Score / Noul
       |
       v
deterministic PolicyGate
       |
       v
bounded Kubernetes action
```

The model interprets state. The controller decides whether anything is allowed to happen.

---

## Real failure classes Jevlet is intended to reason about

### 1. Affinity / anti-affinity makes a Deployment unschedulable

Example:

```yaml
replicas: 3

affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
    - labelSelector:
        matchLabels:
          app: backend
      topologyKey: kubernetes.io/hostname
```

If only two eligible workers exist, two replicas run and the third remains `Pending`.

There may be no useful application logs because the application never started.

Useful state looks more like:

```json
{
  "workload": "backend",
  "desiredReplicas": 3,
  "availableReplicas": 2,
  "eligibleNodes": 2,
  "pendingPods": 1,
  "schedulerReasons": [
    "didn't match pod anti-affinity rules"
  ],
  "nodePressure": false,
  "imagePullFailures": false
}
```

A bounded Jev decision can classify:

```text
scheduling_constraint
resource_exhaustion
image_failure
storage_constraint
unknown
```

Jev is not asked to rewrite the affinity rule. It is asked what failure class the evidence supports.

---

### 2. Network bottleneck while every Pod is Running

A cluster can look healthy from `kubectl get pods`:

```text
Running
Running
Running
Running
```

while users see multi-second latency.

Useful state might include:

```json
{
  "readyPods": 4,
  "podRestarts": 0,
  "cpuUtilization": 0.37,
  "memoryUtilization": 0.44,
  "httpP95Ms": 4800,
  "backendProcessingP95Ms": 220,
  "gatewayP95Ms": 4650,
  "packetDrops5m": 1842,
  "tcpRetransmits5m": 927,
  "dbP95Ms": 31
}
```

That gives Jev enough bounded evidence to distinguish:

```text
application_failure
database_failure
network_path
resource_pressure
unknown
```

without dumping thousands of log lines into a model.

---

### 3. Namespace deleted: there are no broken Pods left to alert on

Suppose someone deletes:

```bash
kubectl delete namespace payments
```

Eventually the namespace, Deployments, Pods, Services, Secrets, ConfigMaps, HPAs and NetworkPolicies disappear.

A Pod watcher now sees **nothing**.

The useful state is the difference between expected and observed resources:

```json
{
  "expectedNamespace": "payments",
  "namespaceExists": false,
  "expectedWorkloads": [
    "payments-api",
    "payments-worker"
  ],
  "observedWorkloads": [],
  "gatewayRoutesReferencingNamespace": 2,
  "recentNamespaceDeletionEvent": true
}
```

Absence is state too.

A future Jevlet controller can classify this as `missing_namespace` and escalate even though no unhealthy Pod exists.

---

### 4. Pods are healthy but invisible to a Service

Example:

The Service expects:

```yaml
selector:
  app: backend
```

but a new Helm template emits:

```yaml
labels:
  app.kubernetes.io/name: backend
```

Pods are healthy. The Deployment is available. But the Service has no endpoints.

Useful state:

```json
{
  "service": "backend",
  "serviceSelector": {
    "app": "backend"
  },
  "matchingPods": 0,
  "candidatePods": 2,
  "candidatePodLabels": {
    "app.kubernetes.io/name": "backend"
  },
  "endpointCount": 0,
  "podReadiness": "healthy"
}
```

This is a much better decision problem than "read the container logs":

```text
service_selector_mismatch
pods_unhealthy
network_policy
port_mismatch
unknown
```

---

### 5. Helm says deployed while dependencies are stale

A Helm release can succeed while the runtime is semantically inconsistent.

Example:

```text
application version: N+1
dependency version:  N
expected API contract: N+1
```

or `Chart.yaml` was changed without refreshing the dependency lock/archive.

Useful state:

```json
{
  "helmRelease": "backend",
  "releaseRevision": 18,
  "releaseStatus": "deployed",
  "applicationImage": "backend:2.8.0",
  "declaredRedisRequirement": ">=8.0",
  "observedRedisVersion": "7.4",
  "chartLockDigestMatches": false,
  "podsReady": true,
  "errorRate": 0.17
}
```

A bounded decision can classify:

```text
stale_release_dependency
application_regression
runtime_dependency_failure
unknown
```

Helm succeeding does not prove that every independently evolving dependency still satisfies the application's assumptions.

---

### 6. NetworkPolicy does exactly what it was told to do

A default-deny egress policy is introduced, but DNS egress is forgotten.

The application starts. Direct IP connectivity may work. DNS fails.

Useful evidence:

```json
{
  "namespaceDefaultDenyEgress": true,
  "dnsEgressAllowed": false,
  "kubeDnsEndpointsHealthy": true,
  "targetServiceEndpointsHealthy": true,
  "dnsResolutionProbe": false,
  "tcpDirectIPProbe": true
}
```

That strongly supports:

```text
network_policy_dns_block
```

Again, no giant log-analysis pipeline is required.

---

### 7. A bad rollout where restarting Pods would make things worse

Suppose:

```text
backend-v1: healthy
backend-v2: failing
```

State:

```json
{
  "deploymentGenerationChanged": true,
  "newReplicaSetAgeSec": 150,
  "oldReplicaSetReady": 3,
  "newReplicaSetReady": 1,
  "newReplicaSetRestartRate": 12,
  "oldReplicaSetRestartRate": 0,
  "newVersion5xxRate": 0.31,
  "oldVersion5xxRate": 0.003
}
```

A generic "CrashLoopBackOff -> restart" workflow is not useful here.

The evidence instead supports:

```text
application_regression
```

A future bounded action could be `rollback`, but only if deterministic code verifies that a healthy previous ReplicaSet exists and policy explicitly allows rollback.

---

## Current MVP

The current implementation is deliberately narrower than those target scenarios.

Jevlet watches Pods selected through `JevPolicy` resources and evaluates unhealthy states with TypeSafe Jev:

- `probableCause` — `choice`
- `remediation` — `choice`: `noop`, `escalate`, `restart_pod`
- `operationalRisk` — `score` from 0 through 4
- `safeToExecuteAutomatically` — `noul`

The first release permits only:

```text
NOOP
ESCALATE
RESTART_POD
```

A restart is executed only when all deterministic gates pass:

- policy is in `guarded` mode;
- `restart_pod` is explicitly allowlisted;
- Jev choice confidence meets `minimumConfidence`;
- `safeToExecuteAutomatically` meets `minimumSafeToExecute`;
- risk does not exceed `maximumRiskScore`;
- the Pod is controlled by another workload;
- the namespace is not a Kubernetes system namespace;
- the workload is outside its cooldown window.

Every decision becomes a `JevIncident` receipt before any action is taken.

---

## Security boundary

Jevlet deliberately does **not** expose arbitrary command execution to Jev.

Jev cannot return:

```bash
kubectl delete pod backend --force
```

It cannot generate arbitrary Kubernetes YAML.

It cannot choose an arbitrary namespace or object name.

The controller already knows which resource it is reconciling. Jev can only return a bounded decision such as:

```text
remediation = restart_pod
confidence = 0.95
```

The deterministic policy gate decides whether that action is allowed.

Automatic mutation currently supports only Pod deletion with a UID precondition, allowing the owning Deployment or StatefulSet to recreate the Pod without giving Jev arbitrary Kubernetes API authority.

---

## Decision receipts

Every evaluation becomes a `JevIncident`.

The long-term goal is for each receipt to preserve enough information to answer:

- What resource was observed?
- What state was supplied?
- Which Jev model/version made the decision?
- What were the Choice/Score/Noul outputs?
- Which policy version evaluated them?
- Was the action approved?
- Was it executed?
- What happened afterward?

This makes it possible to build a replayable dataset of real operational decisions and measure:

```text
classification accuracy
false automatic-remediation rate
human escalation rate
action success rate
confidence calibration
decision latency
API cost
```

---

## Optional Prometheus context

A `JevPolicy` can add bounded Prometheus instant queries.

Query templates support only `{{namespace}}` and `{{pod}}`.

```yaml
signals:
  prometheus:
    url: http://kube-prometheus-stack-prometheus.monitoring.svc:9090
    queries:
      restartRate: 'sum(increase(kube_pod_container_status_restarts_total{namespace="{{namespace}}",pod="{{pod}}"}[5m]))'
```

Prometheus failures are non-fatal. Jevlet records the signal error and continues with Kubernetes evidence.

The design rule is simple:

> Do not dump telemetry into the model. Select bounded evidence for a specific decision.

---

## Quick start

Prerequisites:

- Kubernetes 1.35+
- `kubectl`
- TypeSafe API key
- an image containing Jevlet

Install:

```bash
kubectl apply -k config/default

kubectl -n jevlet-system create secret generic jevlet-typesafe \
  --from-literal=api-key="$TYPESAFE_API_KEY"
```

For local development using the current kubeconfig:

```bash
export TYPESAFE_API_KEY='...'
export JEV_MODEL='jev-latest'

go run ./cmd/manager
```

Create a policy:

```bash
kubectl apply -f config/samples/jevlet_v1alpha1_jevpolicy.yaml
```

Observe decision receipts:

```bash
kubectl get jevincidents -A
kubectl get jevincident -n default -o yaml
```

---

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `TYPESAFE_API_KEY` | required | TypeSafe bearer token |
| `JEV_MODEL` | `jev-latest` | TypeSafe model or alias |
| `JEV_BASE_URL` | `https://api.typesafe.ai` | API base URL |
| `METRICS_BIND_ADDRESS` | `:8080` | Prometheus metrics endpoint |
| `HEALTH_PROBE_BIND_ADDRESS` | `:8081` | health/readiness probes |
| `LEADER_ELECT` | `false` | controller-runtime leader election |

---

## Metrics

Jevlet exposes controller-runtime metrics plus:

- `jevlet_decisions_total{action,approved}`
- `jevlet_decision_latency_seconds`
- `jevlet_api_errors_total`
- `jevlet_actions_total{action,result}`

---

## Roadmap

The interesting roadmap is not "give the AI more tools."

It is expanding the set of Kubernetes failure classes that can be represented safely:

- scheduler constraints: affinity, anti-affinity, topology spread, taints/tolerations;
- Service/Endpoint selector mismatches;
- NetworkPolicy reachability;
- namespace/resource disappearance;
- Gateway API health and progressive delivery;
- Helm release/dependency drift;
- rollout comparison against previous ReplicaSets;
- bounded network signals from Prometheus;
- Alertmanager context;
- bounded Loki error fingerprints;
- Vault and external dependency health;
- Deployment rollback;
- bounded scaling;
- Gateway API canary actions: `continue`, `hold`, `rollback`;
- offline decision replay;
- calibration and policy regression testing;
- signed decision receipts.

Each new automatic action should require its own deterministic policy contract.

---

## Development

```bash
make fmt
make test
make build
```

The checked-in CRDs and RBAC allow the project to run without code generation. GitHub Actions runs formatting checks, `go vet`, and tests.

---

## The experiment

Jevlet is **not** trying to become an autonomous Kubernetes SRE.

The narrower question is:

> Can a fast typed decision model fill the semantic gap between raw Kubernetes state and deterministic controllers?

Kubernetes already reconciles state extremely well.

Prometheus already collects numerical evidence.

The scheduler already explains why it could not place a Pod.

Helm already knows what it installed.

Gateway API already represents traffic policy.

The missing layer is often the bounded interpretation that says:

```text
these facts look like a scheduling constraint

these facts look like a broken Service selector

these facts look like a network path failure

these facts look like dependency drift

these facts look like a bad rollout rather than a random broken Pod
```

That is the hypothesis behind Jevlet.
