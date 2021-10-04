#!/usr/bin/env bash
set -euo pipefail

KUBE="k3s"
SERVER=$(hostname)
WEB_PORT="24817"
if [[ "$1" == "--minikube" ]] || [[ "$1" == "-m" ]]; then
  KUBE="minikube"
  SERVER="localhost"
  if [[ "$CI_TEST" == "true" ]]; then
    SVC_NAME="example-pulp-web-svc"
    WEB_PORT="24880"
    kubectl port-forward service/$SVC_NAME $WEB_PORT:$WEB_PORT &
  fi
fi

pip install ansible

BASE_ADDR="http://$SERVER:$WEB_PORT"
echo $BASE_ADDR
echo "Base Address: $BASE_ADDR"
REPOS=( "published" "staging" "rejected" "community" "rh-certified" )
REPO_RESULTS=()

echo "Waiting ..."
sleep 10

TOKEN=$(curl --location --request POST "$BASE_ADDR/api/galaxy/v3/auth/token/" --header 'Authorization: Basic YWRtaW46cGFzc3dvcmQ=' --silent | python3 -c "import sys, json; print(json.load(sys.stdin)['token'])")
echo $TOKEN

echo "Testing ..."

for repo in "${REPOS[@]}"
do
	# echo $repo
    COLLECTION_URL="$BASE_ADDR/api/galaxy/content/$repo/v3/collections/"
    # echo $COLLECTION_URL
    HTTP_CODE=$(curl --write-out "%{http_code}\n" -H "Authorization:Token $TOKEN" $COLLECTION_URL --silent --output /dev/null)
    # echo $HTTP_CODE
    REPO_RESULTS+=($HTTP_CODE)
done

GALAXY_INIT_RESULT=0
ITER=0
for code in "${REPO_RESULTS[@]}"
do
    echo "${REPOS[$ITER]} $code"
    ITER=$((ITER + 1))
    if [[ $code != 200 ]]; then
        GALAXY_INIT_RESULT=$ITER
    fi
done

cat >> ansible.cfg << ANSIBLECFG
[defaults]
remote_tmp     = /tmp/ansible
local_tmp      = /tmp/ansible

[galaxy]
server_list = community_repo

[galaxy_server.community_repo]
url=${BASE_ADDR}/api/galaxy/content/inbound-community/
token=${TOKEN}
ANSIBLECFG

# Poll a Pulp task until it is finished.
wait_until_task_finished() {
    echo "Polling the task until it has reached a final state."
    local task_url=$1
    while true
    do
        response=$(curl "$task_url")
        local response
        state=$(jq -r .state <<< "${response}")
        local state
        jq . <<< "${response}"
        case ${state} in
            failed|canceled)
                echo "Task in final state: ${state}"
                exit 1
                ;;
            completed)
                echo "$task_url complete."
                break
                ;;
            *)
                echo "Still waiting..."
                sleep 1
                ;;
        esac
    done
}


echo "Creating community namespace"
curl -X POST -d '{"name": "community", "groups":[]}' -H 'Content-Type: application/json' -H 'Accept: application/json' -H "Authorization:Token $TOKEN" $BASE_ADDR/api/galaxy/v3/namespaces/

echo "Upload community.kubernetes collection"
ansible-galaxy collection publish -vvvv -c ./vendor/galaxy.ansible.com/community/kubernetes/community-kubernetes-1.2.1.tar.gz

echo "Check if it was uploaded"
curl -H "Authorization:Token $TOKEN" $BASE_ADDR/api/galaxy/content/staging/v3/collections/ | jq

echo "Sync collections"
curl -X PUT -d '{"requirements_file": "collections: \n - pulp.pulp_installer", "url": "https://galaxy.ansible.com/api/"}' -H 'Content-Type: application/json' -H 'Accept: application/json' -H "Authorization:Token $TOKEN" $BASE_ADDR/api/galaxy/content/community/v3/sync/config/ | jq
TASK_PK=$(curl -X POST -H "Authorization:Token $TOKEN" $BASE_ADDR/api/galaxy/content/community/v3/sync/ | jq -r '.task')
echo "$BASE_ADDR/pulp/api/v3/tasks/$TASK_PK/"
wait_until_task_finished "$BASE_ADDR/pulp/api/v3/tasks/$TASK_PK/"

echo "Install pulp.pulp_installer collection"
mkdir -p /tmp/ci_test
sed -i "s/inbound-community/community/g" ansible.cfg
ansible-galaxy collection install -vvvv pulp.pulp_installer -c -p /tmp/ci_test
tree /tmp/ci_test

exit $GALAXY_INIT_RESULT
