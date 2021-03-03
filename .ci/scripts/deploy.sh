#!/bin/bash -e
#!/usr/bin/env bash

KUBE_FLAG=""
if
  [ "$1" = "--minikube" ] || [ "$1" = "-m" ]; then
  KUBE_FLAG="-m"
fi


echo "Deploy pulp-operator"
# sudo -E operator-sdk build quay.io/pulp/pulp-operator:latest
eval $(minikube -p minikube docker-env)
sudo -E $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh
