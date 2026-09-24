# Jevlet

**Jevlet is a System One control loop for Kubernetes.** It turns Kubernetes state, events, and optional Prometheus signals into typed Jev decisions, then passes those decisions through deterministic policy before any mutation is allowed.

> Jev decides. Jevlet authorizes and executes.

Jevlet never asks an AI model to generate shell commands or arbitrary Kubernetes objects. The model is restricted to a closed decision vocabulary; ordinary Go code owns authorization and actuation.

## MVP

The first release watches Pods selected by `JevPolicy` resources and evaluates unhealthy states with TypeSafe Jev:

- `probableCause` — `choice`
- `remediation` — `choice`: `noop`, `escalate`, `restart_pod`
- `operationalRisk` — `score` from 0 through 4
- `safeToExecuteAutomatically` — `noul`

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

## Architecture

```text
Pod / Events / Prometheus
          |
          v
      StateBuilder
          |
          v
  TypeSafe Jev API
 Choice + Score + Noul
          |
          v
 deterministic PolicyGate
          |
     +----+---------+
     |              |
  observe        execute
                    |
                    v
             Kubernetes API
```

## Quick start

Prerequisites: Kubernetes 1.35+, `kubectl`, a TypeSafe API key, and an image containing Jevlet.

```bash
kubectl apply -k config/default
kubectl -n jevlet-system create secret generic jevlet-typesafe \
  --from-literal=api-key="$TYPESAFE_API_KEY"
```

For a local development run using your current kubeconfig:

```bash
export TYPESAFE_API_KEY='...'
export JEV_MODEL='jev-latest'
go run ./cmd/manager
```

Create a policy in an application namespace:

```bash
kubectl apply -f config/samples/jevlet_v1alpha1_jevpolicy.yaml
```

Observe receipts:

```bash
kubectl get jevincidents -A
kubectl get jevincident -n default -o yaml
```

## Optional Prometheus context

A policy can add bounded Prometheus instant queries. Query templates support only two substitutions: `{{namespace}}` and `{{pod}}`.

```yaml
signals:
  prometheus:
    url: http://kube-prometheus-stack-prometheus.monitoring.svc:9090
    queries:
      restartRate: 'sum(increase(kube_pod_container_status_restarts_total{namespace="{{namespace}}",pod="{{pod}}"}[5m]))'
```

Prometheus failures are non-fatal: Jevlet records the signal error in state and continues with Kubernetes evidence.

## Security boundary

Jevlet deliberately does **not** expose arbitrary command execution to Jev. The model can return only values from an allowlisted schema. The controller then evaluates those values against Kubernetes-native policy. Automatic mutations currently support only Pod deletion with UID preconditions, which lets the owning Deployment/StatefulSet recreate the Pod without allowing Jev to choose a resource name or API operation.

## Configuration

Environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `TYPESAFE_API_KEY` | required | TypeSafe bearer token |
| `JEV_MODEL` | `jev-latest` | TypeSafe model or alias |
| `JEV_BASE_URL` | `https://api.typesafe.ai` | API base URL |
| `METRICS_BIND_ADDRESS` | `:8080` | Prometheus metrics endpoint |
| `HEALTH_PROBE_BIND_ADDRESS` | `:8081` | health/readiness probes |
| `LEADER_ELECT` | `false` | controller-runtime leader election |

## Metrics

Jevlet exposes controller-runtime metrics plus:

- `jevlet_decisions_total{action,approved}`
- `jevlet_decision_latency_seconds`
- `jevlet_api_errors_total`
- `jevlet_actions_total{action,result}`

## Roadmap

- Loki evidence adapter with strict byte/time budgets.
- Deployment rollback and bounded scale actions.
- Gateway API progressive-delivery decisions (`continue`, `hold`, `rollback`).
- Alertmanager context.
- Jev policy versioning and signed decision receipts.
- Offline replay/evaluation harness for calibration and policy regression tests.

## Development

```bash
make fmt
make test
make build
```

The checked-in CRDs and RBAC allow the project to run without code generation. GitHub Actions runs formatting checks, `go vet`, and tests.
