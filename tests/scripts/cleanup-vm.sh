#!/bin/bash

source scripts/vars.env

# Remove VM IP from GKE master control node access, if required
if [ "${ADD_VM_IP_AUTH_NETWORKS}" = "true" ]; then
    EXTERNAL_IP=$(gcloud compute instances describe ${RESOURCE_NAME} --project=${GKE_PROJECT} --zone=${GKE_CLUSTER_ZONE} \
                    --format='value(networkInterfaces[0].accessConfigs[0].natIP)')
    CURRENT_AUTH_NETWORK=$(gcloud container clusters describe ${GKE_CLUSTER_NAME} \
                            --format="value(masterAuthorizedNetworksConfig.cidrBlocks[0])" | sed 's/cidrBlock=//')
    gcloud container clusters update ${GKE_CLUSTER_NAME} --enable-master-authorized-networks --master-authorized-networks=${CURRENT_AUTH_NETWORK}
fi

gcloud compute instances delete ${RESOURCE_NAME} --quiet --project=${GKE_PROJECT} --zone=${GKE_CLUSTER_ZONE}
gcloud compute firewall-rules delete ${RESOURCE_NAME} --quiet --project=${GKE_PROJECT}
