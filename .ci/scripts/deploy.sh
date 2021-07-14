#!/bin/bash -e
#!/usr/bin/env bash

export QUAY_IMAGE_TAG=$(python -c 'import yaml; print(yaml.safe_load(open("deploy/olm-catalog/pulp-operator/manifests/pulp-operator.clusterserviceversion.yaml"))["spec"]["version"])')
echo $QUAY_IMAGE_TAG
#sed -i "s/\.dev//g" deploy/olm-catalog/pulp-operator/manifests/pulp-operator.clusterserviceversion.yaml
cat deploy/olm-catalog/pulp-operator/manifests/pulp-operator.clusterserviceversion.yaml | grep "pulp-operator.v"
docker build -f bundle.Dockerfile -t quay.io/pulp/pulp-operator-bundle:${QUAY_IMAGE_TAG} .
# sudo -E QUAY_REPO_NAME=pulp-operator-bundle $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh


wget https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest-4.7/opm-linux.tar.gz
tar xvf opm-linux.tar.gz
sudo mv opm /usr/local/bin/opm
sudo chmod +x /usr/local/bin/opm


opm index add -c docker --bundles quay.io/pulp/pulp-operator-bundle:${QUAY_IMAGE_TAG} --tag quay.io/pulp/pulp-index:${QUAY_IMAGE_TAG}
sudo -E QUAY_REPO_NAME=pulp-index $GITHUB_WORKSPACE/.ci/scripts/quay-push.sh

docker images
