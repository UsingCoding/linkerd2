# Core concept

Proxy supervisor delays termination of linkerd-proxy container until other containers in pod is not terminated.

# Problem

Solves problem in pod termination process when linkerd-proxy exit earlier than main container,
but main container still handles request and need to perform network request after proxy container already exited
which leads to errors in application

# Native solution

Kubernetes v1.28 introduced [native sidecars](https://kubernetes.io/blog/2023/08/25/native-sidecar-containers/),
and they solve [problem](#problem) in more canonical way - [When to use sidecar containers](https://kubernetes.io/blog/2023/08/25/native-sidecar-containers/#when-to-use-sidecar-containers).

So if your kubernetes version >=1.28 it's **strongly recommended** to use `Native sidecars` instead of this supervisor.

This solution for users of linkerd with lower versions of kubernetes.

# Solution

Small Go-supervisor which tracks status of containers and terminate linkerd-proxy after all containers exited.

## Algorithm explanation

* Supervisor launches linkerd-proxy
* Supervisor start tracking container statuses
* Supervisor wait for termination signal from K8S
* `kubectl roolout restart deployment <deployment>`
* Supervisor receives termination signal from kubelet and wait:
  * Until all containers estimated stopped
  * Until [timeout](#graceful-termination-timeout) is not expires
* When one of conditions satisfied: supervisor terminates linkerd-proxy by sending `SIGTERM`

# Requirements

ServiceAccount with following role:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: linkerd-supervisor
rules:
  - apiGroups:
      - ""
    resources:
      - pods
    verbs:
      - get
      - list
      - watch
```

# Core principles

## Configuration state

* By default, supervisor disabled. Enable it with env `LINKERD2_PROXY_SUPERVISOR=1`
* [Graceful termination](#graceful-termination-timeout), default is `30s`. Should be coupled with
* Log format: use same configuration as linkerd-proxy

## Status tracking

Track status via k8s API and watching pod changes.
See -> `pkg/supervisor/kubewatch`

## Graceful termination timeout

In some cases k8s API server may not to deliver event about containers in pod,
so after receiving termination signal from kubelet supervisor launches `graceful-termination-timeout`
until terminate proxy manually.

By default, it coupled with [k8s terminationGracePeriodSeconds](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#probe-level-terminationgraceperiodseconds) and equal to `30s`.
It **should be** coupled or less than `terminationGracePeriodSeconds`. Otherwise, kubelet terminate container earlier.
