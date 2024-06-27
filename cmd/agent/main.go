package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/t-chdossa_microsoft/aks-mesh/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
	_ "k8s.io/client-go/plugin/pkg/client/auth" 
)

func main() {
	/*
		TODO:
			- ensure I have a wireguard interface
			- ensure I am peered with the gateways
			- ensure I create a Peer resource in the k8s cluster that has:
				- my public key
				- my pod IPs
				- my reachable endpoint (just node ip)
	*/
	ensureWireGuardInterface()
	ensurePeeringWithGateways()
	createPeerResource()
	fmt.Println("Completed setup.")
}

func runCommand(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	return string(out), err
}

func ensureWireGuardInterface() {
	fmt.Println("Ensuring WireGuard interface...")

	cmd := "ip link show dev wg0"
	_, err := runCommand(cmd)
	if err != nil {
		fmt.Println("Creating WireGuard interface...")
		cmd = "ip link add dev wg0 type wireguard"
		_, err = runCommand(cmd)
		if err != nil {
			fmt.Println("Error creating WireGuard interface: ", err)
			return
		}

		// Configure the WireGuard interface
		cmd = "ip address add dev wg0 your_ip_address/24"
		_, err = runCommand(cmd)
		if err != nil {
			fmt.Println("Error adding IP address to WireGuard interface: ", err)
			return
		}

		// Bring up the WireGuard interface
		cmd = "ip link set up dev wg0"
		_, err = runCommand(cmd)
		if err != nil {
			fmt.Println("Error bringing up WireGuard interface: ", err)
			return
		}

		// Add WireGuard configuration here (if needed)
		fmt.Println("WireGuard interface created and configured.")
	} else {
		fmt.Println("WireGuard interface already exists.")
	}
}

func ensurePeeringWithGateways() {
	fmt.Println("Ensuring peering with gateways...")

	// Load kubeconfig
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		log.Fatalf("Unable to find kubeconfig")
	}

	// Build the client config
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	// Create the client
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		log.Fatalf("Error adding Gateway schema to scheme: %v", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	// Fetch Gateway list
	var gatewayList v1alpha1.GatewayList
	if err := k8sClient.List(context.Background(), &gatewayList, &client.ListOptions{}); err != nil {
		log.Fatalf("Error fetching Gateways: %v", err)
	}

	// Process each gateway and configure peering
	for _, gateway := range gatewayList.Items {
		fmt.Printf("Configuring peering with gateway: %s (Endpoint: %s, PublicKey: %s)\n", gateway.Name, gateway.Spec.Endpoint, gateway.Spec.PublicKey)

		// Add your WireGuard peering logic here using the gateway.Spec.Endpoint and gateway.Spec.PublicKey
		_, err := runCommand(fmt.Sprintf("wg set wg0 peer %s endpoint %s", gateway.Spec.PublicKey, gateway.Spec.Endpoint))
		if err != nil {
			log.Fatalf("Error configuring peering with gateway %s: %v", gateway.Name, err)
		}
	}

	fmt.Println("Peering with gateways ensured.")
}

func createPeerResource() {
	fmt.Println("Creating Peer resource...")

	// Load kubeconfig
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		log.Fatalf("Unable to find kubeconfig")
	}

	// Build the client config
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	// Retrieve node IP
	nodeName, err := getNodeName()
	if err != nil {
		log.Fatalf("Error getting node name: %v", err)
	}

	nodeIP, err := getNodeIP(clientset, nodeName)
	if err != nil {
		log.Fatalf("Error getting node IP: %v", err)
	}

	// Retrieve pod IPs
	podIPs, err := getPodIPs(clientset, nodeName)
	if err != nil {
		log.Fatalf("Error getting pod IPs: %v", err)
	}

	// Retrieve WireGuard public key
	publicKey, err := getWireGuardPublicKey()
	if err != nil {
		log.Fatalf("Error getting WireGuard public key: %v", err)
	}

	// Define the Peer resource
	peer := &v1alpha1.Peer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-peer",
		},
		Spec: v1alpha1.PeerSpec{
			PublicKey: publicKey,
			PodIPs:    podIPs,
			Endpoint:  nodeIP,
		},
	}

	// Create the Peer resource
	_, err = clientset.CoreV1().RESTClient().
		Post().
		Namespace("default").
		Resource("peers").
		Body(peer).
		Do(context.Background()).
		Get()

	if err != nil {
		log.Fatalf("Error creating Peer resource: %v", err)
	}

	fmt.Println("Peer resource created successfully.")
}

func getNodeName() (string, error) {
	cmd := "hostname"
	nodeName, err := runCommand(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(nodeName), nil
}

func getNodeIP(clientset *kubernetes.Clientset, nodeName string) (string, error) {
	node, err := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address, nil
		}
	}

	return "", fmt.Errorf("node internal IP not found")
}

func getPodIPs(clientset *kubernetes.Clientset, nodeName string) ([]string, error) {
	podList, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return nil, err
	}

	var podIPs []string
	for _, pod := range podList.Items {
		podIPs = append(podIPs, pod.Status.PodIP)
	}

	return podIPs, nil
}

func getWireGuardPublicKey() (string, error) {
	cmd := "cat /etc/wireguard/publickey"
	publicKey, err := runCommand(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(publicKey), nil
}
