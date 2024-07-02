#!/bin/sh

# Ensure that POD_CIDR is set
if [ -z "$POD_CIDR" ]; then
  echo "POD_CIDR environment variable is not set. Exiting."
  exit 1
fi

# Ensure that POD_IP is set
if [ -z "$POD_IP" ]; then
  echo "POD_IP environment variable is not set. Exiting."
  exit 1
fi

# Ensure that KUBECONFIG is set
if [ -z "$KUBECONFIG" ]; then
  echo "KUBECONFIG environment variable is not set. Exiting."
  exit 1
fi

# Start the gateway application with the pod-cidr flag
/app/gateway --pod-cidr=$POD_CIDR &

# Start the agent application
/app/agent &

# Wait for all background processes to finish
wait
