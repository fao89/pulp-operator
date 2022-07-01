#!/bin/bash -e
#!/usr/bin/env bash

if [[ "$CI_TEST" == "galaxy" ]]; then
  echo "Deploy galaxy latest"
   QUAY_REPO_NAME=galaxy $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh
  echo "Deploy galaxy-web latest"
   QUAY_REPO_NAME=galaxy-web $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh
else
  echo "Deploy pulp latest"
   QUAY_REPO_NAME=pulp $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh

  echo "Deploy pulpcore latest"
   QUAY_REPO_NAME=pulpcore $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh

  echo "Deploy pulp-web latest"
   QUAY_REPO_NAME=pulp-web $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh
fi

if [[ -z "${QUAY_EXPIRE+x}" ]]; then
  echo "Deploy pulp-operator"
  make docker-push

  export QUAY_IMAGE_TAG=v$(cat Makefile | grep "VERSION ?=" | cut -d' ' -f3)
  echo $QUAY_IMAGE_TAG
  podman tag quay.io/pulp/pulp-operator:devel quay.io/pulp/pulp-operator:$QUAY_IMAGE_TAG
   $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh

  make bundle-build
  make bundle-push

  make catalog-build
  make catalog-push
  sudo -E podman images
fi

sudo -E podman images
