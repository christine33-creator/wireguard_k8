#!/bin/bash

# Default namespaces
NAMESPACE="default"
UNDERLAY_CONTEXT=""
OVERLAY_CONTEXT=""

# Function to print usage
usage() {
    echo "Usage: $0 -n <namespace> -u <underlay_context> -o <overlay_context>"
    echo "Options:"
    echo "  -n, --namespace <namespace>         Namespace in which Konnectivity is deployed (default: default)"
    echo "  -u, --underlay-context <context>    Underlay context"
    echo "  -o, --overlay-context <context>     Overlay context"
    exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        -n|--namespace)
            NAMESPACE="$2"
            shift
            shift
            ;;
        -u|--underlay-context)
            UNDERLAY_CONTEXT="$2"
            shift
            shift
            ;;
        -o|--overlay-context)
            OVERLAY_CONTEXT="$2"
            shift
            shift
            ;;
        *)
            # Print usage if unknown option
            usage
            ;;
    esac
done

# Check if required arguments are provided
if [ -z "$UNDERLAY_CONTEXT" ] || [ -z "$OVERLAY_CONTEXT" ]; then
    echo "Error: Underlay and overlay contexts are required. Exiting."
    usage
fi

# Content to replace in helmvalues_konnectivity.go
NEW_CONTENT=$(cat <<EOF
package helmvalues

import (
        "context"
        "go.goms.io/aks/rp/overlaymgr/server/helmctlr/helmvalues/types"
        "go.goms.io/aks/rp/overlaymgr/server/log"
        hcpCPW "go.goms.io/aks/rp/protos/hcp/types/controlplanewrapper/v1"
        hcpAgentPool "go.goms.io/aks/rp/protos/hcp/types/agentpool/v1"
)

func konnectivityValues(ctx context.Context, g *types.Globals, e types.Entity) *types.KonnectivityValues {
        // Log the disabling of Konnectivity.
        log.FromCtx(ctx).Infof(ctx, "Konnectivity is disabled")

        // Return nil or an empty struct, depending on the type definition.
        return nil // or return struct{}{} if it's a struct type.
}
EOF
)

# Step 1: Navigate to Konnectivity helm deployment file
export KUBECONFIG="$UNDERLAY_CONTEXT"
cd overlaymgr/server/helmctlr/helmvalues/ || { echo "Error: Unable to navigate to helm values directory. Exiting."; exit 1; }

# Step 2: Modify the helmvalues_konnectivity.go file to disable Konnectivity
echo "Disabling Konnectivity..."
echo "$NEW_CONTENT" | sudo tee helmvalues_konnectivity.go > /dev/null || { echo "Error: Unable to modify helmvalues_konnectivity.go. Exiting."; exit 1; }

# Step 3: Delete Konnectivity service in the underlay context
echo "Deleting Konnectivity service in the underlay context..."
kubectl delete svc konnectivity -n "$NAMESPACE" || { echo "Error: Unable to delete Konnectivity service. Exiting."; exit 1; }

# Step 4: Scale down the addon-manager deployment to zero replicas
echo "Scaling down addon-manager deployment..."
ccp_namespace=$(kubectl get namespace | grep -E "^${NAMESPACE}\s" | awk '{print $1}')
if [ -z "$ccp_namespace" ]; then
    echo "Error: Namespace '$NAMESPACE' not found. Exiting."
    exit 1
fi
kubectl scale -n "$ccp_namespace" deployments/kube-addon-manager --replicas=0 || { echo "Error: Unable to scale down addon-manager deployment. Exiting."; exit 1; }

# Step 5: Delete Konnectivity Network Policy
echo "Deleting Konnectivity Network Policy..."
export KUBECONFIG="$OVERLAY_CONTEXT"
kubectl delete deployment konnectivity-agent -n kube-system || { echo "Error: Unable to delete konnectivity-agent deployment. Exiting."; exit 1; }
kubectl delete deployment konnectivity-agent-autoscaler -n kube-system || { echo "Error: Unable to delete konnectivity-agent-autoscaler deployment. Exiting."; exit 1; }
kubectl delete networkpolicy konnectivity-agent -n kube-system || { echo "Error: Unable to delete konnectivity-agent Network Policy. Exiting."; exit 1; }

echo "Konnectivity successfully disabled and related resources deleted."
