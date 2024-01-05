#!/bin/bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

source scripts/vars.env

gcloud compute firewall-rules create ${FIREWALL_RULE_NAME} \
    --project=${GKE_PROJECT} \
    --direction=INGRESS \
    --priority=1000 \
    --network=default \
    --action=ALLOW \
    --rules=tcp:22 \
    --source-ranges=${SOURCE_IP_RANGE} \
    --target-tags=${NETWORK_TAGS}

gcloud compute instances create ${VM_NAME} --project=${GKE_PROJECT} --zone=${GKE_CLUSTER_ZONE} --machine-type=e2-medium \
    --network-interface=network-tier=PREMIUM,stack-type=IPV4_ONLY,subnet=default --maintenance-policy=MIGRATE \
    --provisioning-model=STANDARD --service-account=${GKE_SVC_ACCOUNT} \
    --scopes=https://www.googleapis.com/auth/devstorage.read_only,https://www.googleapis.com/auth/logging.write,https://www.googleapis.com/auth/monitoring.write,https://www.googleapis.com/auth/servicecontrol,https://www.googleapis.com/auth/service.management.readonly,https://www.googleapis.com/auth/trace.append,https://www.googleapis.com/auth/cloud-platform \
    --tags=${NETWORK_TAGS} --create-disk=auto-delete=yes,boot=yes,device-name=${VM_NAME},image=${IMAGE},mode=rw,size=10 --no-shielded-secure-boot --shielded-vtpm --shielded-integrity-monitoring --labels=goog-ec-src=vm_add-gcloud --reservation-affinity=any

# Add VM IP to GKE master control node access, if required
if [ "${ADD_VM_IP_AUTH_NETWORKS}" = "true" ]; then
    EXTERNAL_IP=$(gcloud compute instances describe ${VM_NAME} --project=${GKE_PROJECT} --zone=${GKE_CLUSTER_ZONE} \
                    --format='value(networkInterfaces[0].accessConfigs[0].natIP)')
    CURRENT_AUTH_NETWORK=$(gcloud container clusters describe ${GKE_CLUSTER_NAME} \
                            --format="value(masterAuthorizedNetworksConfig.cidrBlocks[0])" | sed 's/cidrBlock=//')
    gcloud container clusters update ${GKE_CLUSTER_NAME} --enable-master-authorized-networks --master-authorized-networks=${CURRENT_AUTH_NETWORK},${EXTERNAL_IP}/32
fi

# Poll for SSH connectivity
MAX_RETRIES=30
RETRY_INTERVAL=10
for ((i=1; i<=MAX_RETRIES; i++)); do
    echo "Attempt $i to connect to the VM..."
    gcloud compute ssh ${VM_NAME} --zone=${GKE_CLUSTER_ZONE} --project=${GKE_PROJECT} --quiet --command="echo 'VM is ready'"
    if [ $? -eq 0 ]; then
        echo "SSH connection successful. VM is ready."
        break
    fi
    echo "Waiting for ${RETRY_INTERVAL} seconds before the next attempt..."
    sleep ${RETRY_INTERVAL}
done

gcloud compute scp --zone ${GKE_CLUSTER_ZONE} --project=${GKE_PROJECT} ${SCRIPT_DIR}/vars.env ${VM_NAME}:~

gcloud compute ssh --zone ${GKE_CLUSTER_ZONE} --project=${GKE_PROJECT} ${VM_NAME} --command="bash -s" < ${SCRIPT_DIR}/remote-scripts/install-deps.sh
