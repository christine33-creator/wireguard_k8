# AKS Mesh: A WireGuard-based Kubernetes Network Policy

**AKS Mesh** creates a secure, encrypted VPN communication mesh within a Kubernetes cluster using WireGuard. By leveraging Konnectivity and operating in host-network mode, it offers enhanced flexibility and performance while maintaining compatibility with existing setups.

### Understanding Wireguard and Konnectivity

**WireGuard** is a modern, open-source VPN technology known for its simplicity, fast performance, and strong security. It uses state-of-the-art cryptography and efficient algorithms to establish secure tunnels between endpoints.

**Konnectivity** is a Kubernetes network plugin that provides network connectivity for Pods without relying on traditional CNI plugins. It allows Pods to communicate directly with services and other Pods, even when they are not in the same network namespace.

**AKS Mesh** combines these technologies to create a highly performant and secure overlay network policy within a Kubernetes cluster.

### How it works

AKS Mesh establishes a mesh network by:

1. Deploying WireGuard gateways as sidecars with Konnectivity agents.
2. Creating WireGuard interfaces on each node for secure communication.
3. Managing peer relationships through custom Kubernetes resources.
   
**Key Components**:

- **Gateway Service**: Handles incoming traffic from Konnectivity agents and forwards it to the appropriate nodes.
- **Node Agent**: Runs on each node to manage WireGuard connections and peer relationships.

<img src="https://github.com/christine33-creator/wireguard_k8/assets/119143674/62b7b3b8-de71-4603-abcc-89a266e61710" width="500" height="400">


### Benefits

- **Secure communication**: Encrypted VPN tunnels using WireGuard.
- **Host-network mode**: Enhanced performance and flexibility.
- **Konnectivity integration**: Leveraging existing benefits and compatibility.
- **Kubernetes-native management**: Using custom resources for configuration.

## Getting Started

## Prerequisites

- Kubernetes cluster
- Docker
- kubectl configured

## Installation

**1. Clone the repo:**  
`git clone https://github.com/christine33-creator/wireguard_k8
cd aks-mesh`

**2. Build and push the Docker image**  
`docker build --platform="linux/amd64" -t <container-name> --push .`

**3. Deploy the gateway and agents**  
- Deploying the gateway along with konnectivity pods can be achieved by adding a gateway container inside the konnectivity deployment in the pipeline:
```
containers:
      - name: aks-mesh-gateway
        image: <image_name>
        securityContext:
          capabilities:
            add:
              - NET_ADMIN
        command: ["./gateway"]
        env:
          - name: NODE_IP
            valueFrom:
              fieldRef:
                fieldPath: status.hostIP
          - name: NODE_NAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
          - name: POD_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
        args:
          - "--node-name=$(NODE_NAME)"
          - "--gateway-endpoint=$(NODE_IP)"
          - "--pod-cidr={{ range $i, $cidr := .Values.global.commonGlobals.CIDR.ClusterCidrs }}{{- if $i }},{{- end }}{{ $cidr }}{{- end }}"
```

**4. Deploy the CRDs and RBAC**  
Create `peer` and `gateway` CRDs to define the mesh topology.
`kubectl apply -f config/crd/bases`
`kubectl apply -f config/rbac`


**5. Deploy the application in the cluster**  
`kubectl create deployment <deployment-name> --image=<image-name>`


## Use Cases

- **Secure communication between applications**: Establish secure connections between applications running in different Pods or namespaces.
- **Service mesh integration**: Integrate with service mesh solutions for additional traffic management and security features.

## Solution: --agent-identifiers
In this solution, the metrics server pod will now have its own konnectivity agent sidecar. By default, one set of agents handling all traffic. But in this case, the sidecar will ensure direct connectivity between the API server and Metrics Server. The implemention steps are:
- Add `\proxy-agent` container to the metrics server pod
- Set the agent identifier to `ipv4=$POD_IP`
- In this case API server resolves to the IP before connecting to konnectivity, so we can expose the pod IP via the downward API
- Because of the way these agent identifiers work, if that connectivity agent sidecar is broken somehow and can't start, then it will never register itself with connectivity server.

## Troubleshooting

### Common Issues and Solutions

1. **Installation and deployment issues**
   - **Error: Failed to build Docker image**
      - Check the Dockerfile for syntax errors or missing dependencies.
      - Verify the Docker daemon is running and accessible.
      - Ensure sufficient disk space and resources are available.
   - **Error: Failed to deploy CRDs**
      - Verify the Kubernetes cluster is accessible and the kubectl configuration is correct.
      - Check for syntax errors in the CRD manifests.
      - Ensure the Kubernetes API server supports custom resource definitions.
   - **Error: Failed to deploy agent**
      - Verify Docker image permissions and container capabilities.
      - Check for sufficient resource allocation (CPU, memory).
      - Inspect container logs for error messages.
 
2. **Network Connectivity Issues**
   - **Error: Unable to establish WireGuard connections**
      - Verify WireGuard interface configuration (address, port, peers).
      - Check firewall rules to allow WireGuard traffic.
      - Inspect WireGuard logs for error messages.
      - Verify network connectivity between nodes.
   - **Error: Connectivity issues between Pods**
      - Check Konnectivity agent configuration and network policies.
      - Verify Pod network namespaces and IP addresses.
      - Inspect Kubernetes network policies for restrictions.

3. **Configuration Issues**
   - **Error: Invalid Peer or Gateway configuration**
      - Validate custom resource definitions against the schema.
      - Check for typos or incorrect values in configuration files.
      - Verify the specified endpoints and public keys.  
   **- Error: Incorrect WireGuard configuration**
      - Review WireGuard configuration files for errors.
      - Ensure correct peer addresses and public keys.
      - Check for missing or incorrect firewall rules.

