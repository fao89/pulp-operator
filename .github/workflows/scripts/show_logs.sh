#!/bin/bash -e
#!/usr/bin/env bash

KUBE="minikube"
if [[ "$1" == "--kind" ]] || [[ "$1" == "-k" ]]; then
  KUBE="kind"
  echo "Running $KUBE"
fi

sudo -E kubectl get pods -o wide
sudo -E kubectl get deployments

if [[ "$KUBE" == "minikube" ]]; then
  echo ::group::METRICS
  sudo -E kubectl top pods || true
  sudo -E kubectl describe node minikube || true
  echo ::endgroup::
  echo ::group::MINIKUBE_LOGS
  minikube logs -n 10000
  echo ::endgroup::
fi

echo ::group::OPERATOR_LOGS
sudo -E kubectl logs -l app.kubernetes.io/name=pulp-operator -c pulp-manager --tail=10000
echo ::endgroup::

echo ::group::PULP_API_LOGS
sudo -E kubectl logs -l app.kubernetes.io/name=pulp-api --tail=10000
echo ::endgroup::

echo ::group::PULP_CONTENT_LOGS
sudo -E kubectl logs -l app.kubernetes.io/name=pulp-content --tail=10000
echo ::endgroup::

echo ::group::PULP_WORKER_LOGS
sudo -E kubectl logs -l app.kubernetes.io/name=pulp-worker --tail=10000
echo ::endgroup::

echo ::group::PULP_WEB_LOGS
sudo -E kubectl logs -l app.kubernetes.io/name=nginx --tail=10000
echo ::endgroup::

echo ::group::POSTGRES
sudo -E kubectl logs -l app.kubernetes.io/name=postgres --tail=10000
echo ::endgroup::

echo ::group::EVENTS
sudo -E kubectl get events --sort-by='.metadata.creationTimestamp'
echo ::endgroup::

echo ::group::OBJECTS
sudo -E kubectl get pvc,configmap,serviceaccount,secret,networkpolicy,ingress,service,deployment,statefulset,hpa,job,cronjob -o yaml
echo ::endgroup::
