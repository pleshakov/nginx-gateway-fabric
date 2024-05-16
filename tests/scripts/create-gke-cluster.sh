#!/bin/bash

source scripts/vars.env

ip_random_digit=$((1 + $RANDOM % 250))

IS_CI=${1:-false}

if [ -z "$GKE_MACHINE_TYPE" ]; then
    # If the environment variable is not set, use a default value
    GKE_MACHINE_TYPE="e2-medium"
fi

if [ -z "$GKE_NUM_NODES" ]; then
    # If the environment variable is not set, use a default value
    GKE_NUM_NODES="3"
fi

gcloud container clusters create ${GKE_CLUSTER_NAME} \
    --project ${GKE_PROJECT} \
    --zone ${GKE_CLUSTER_ZONE} \
    --enable-master-authorized-networks \
    --enable-ip-alias \
    --service-account ${GKE_NODES_SERVICE_ACCOUNT} \
    --enable-private-nodes \
    --master-ipv4-cidr 172.16.${ip_random_digit}.32/28 \
    --metadata=block-project-ssh-keys=TRUE \
    --monitoring=SYSTEM,POD,DEPLOYMENT \
    --logging=SYSTEM,WORKLOAD \
    --machine-type ${GKE_MACHINE_TYPE} \
    --num-nodes ${GKE_NUM_NODES}

# Add current IP to GKE master control node access, if this script is not invoked during a CI run.
if [ "${IS_CI}" = "false" ]; then
    SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
    bash ${SCRIPT_DIR}/add-local-ip-auth-networks.sh
fi
