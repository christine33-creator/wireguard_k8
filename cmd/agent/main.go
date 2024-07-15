package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/t-chdossa_microsoft/aks-mesh/api/v1alpha1"
	"github.com/t-chdossa_microsoft/aks-mesh/pkg/acn"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
}

type WireGuard struct {
	Attributes *netlink.LinkAttrs
}

func (w *WireGuard) Attrs() *netlink.LinkAttrs {
	return w.Attributes
}

func (w *WireGuard) Type() string {
	return "wireguard"
}

var _ netlink.Link = &WireGuard{}

func main() {
	fmt.Println("Starting WireGuard agent setup...")

	ensureWireGuardInterface()
	ensurePeeringWithGateways()
	createPeerResource()

	fmt.Println("Completed setup.")

	for {
		time.Sleep(2 * time.Second)
		ensurePeeringWithGateways()
	}
}

// func to retrieve node ip dynamically
func getNodeIPAddress() string {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error creating in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	hostname := os.Getenv("NODE_NAME") // Retrieve the node name
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), hostname, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Error getting node %s: %v", hostname, err)
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}

	log.Fatalf("Node internal IP not found for node %s", hostname)
	return ""
}

func ensureWireGuardInterface() {
	fmt.Println("Ensuring WireGuard interface...")

	la := netlink.NewLinkAttrs()
	la.Name = "wg0"
	wgLink := &WireGuard{Attributes: &la}

	err := netlink.LinkAdd(wgLink)
	if err != nil && err.Error() != "file exists" {
		log.Fatalf("Error creating WireGuard interface: %v", err)
	}

	link, err := netlink.LinkByName("wg0")
	if err != nil {
		log.Fatalf("Error getting WireGuard interface: %v", err)
	}

	nodeIP := getNodeIPAddress()
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   net.ParseIP(nodeIP),
			Mask: net.CIDRMask(24, 32),
		},
	}
	err = netlink.AddrAdd(link, addr)
	if err != nil && err.Error() != "file exists" {
		log.Fatalf("Error adding IP address to WireGuard interface: %v", err)
	}

	err = netlink.LinkSetUp(link)
	if err != nil {
		log.Fatalf("Error bringing up WireGuard interface: %v", err)
	}

	fmt.Println("WireGuard interface created and configured.")
}

func ensurePeeringWithGateways() {
	fmt.Println("Ensuring peering with gateways...")

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error creating in-cluster config: %v", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	var gatewayList v1alpha1.GatewayList
	if err := k8sClient.List(context.Background(), &gatewayList); err != nil {
		log.Fatalf("Error fetching Gateways: %v", err)
	}

	cli, err := wgctrl.New()
	if err != nil {
		log.Fatalf("Error creating WireGuard client: %v", err)
	}
	defer cli.Close()

	wgdev, err := cli.Device("wg0")
	if err != nil {
		log.Fatalf("Error getting WireGuard device: %v", err)
	}

	for _, gateway := range gatewayList.Items {
		fmt.Printf("Configuring peering with gateway: %s (Endpoint: %s, PublicKey: %s)\n", gateway.Name, gateway.Spec.Endpoint, gateway.Spec.PublicKey)

		cfg := wgtypes.PeerConfig{
			PublicKey: mustParseKey(gateway.Spec.PublicKey),
			Endpoint:  &net.UDPAddr{IP: net.ParseIP(gateway.Spec.Endpoint), Port: 51820},
			AllowedIPs: []net.IPNet{
				{
					IP:   net.ParseIP(gateway.Spec.Endpoint),
					Mask: net.CIDRMask(24, 32),
				},
			},
		}

		err = cli.ConfigureDevice(wgdev.Name, wgtypes.Config{
			Peers:        []wgtypes.PeerConfig{cfg},
			ReplacePeers: false,
		})
		if err != nil {
			log.Fatalf("Error configuring peering with gateway %s: %v", gateway.Name, err)
		}
	}

	fmt.Println("Peering with gateways ensured.")
}

func createPeerResource() {
	fmt.Println("Creating Peer resource...")

	k8sClient, err := createK8sClient()
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	nodeName := os.Getenv("NODE_NAME") // Use NODE_NAME environment variable
	if nodeName == "" {
		log.Fatalf("NODE_NAME environment variable is not set")
	}

	nodeIP, err := getNodeIP(k8sClient, nodeName)
	if err != nil {
		log.Fatalf("Error getting node IP: %v", err)
	}

	_, publicKey, err := getWireGuardKeys()
	if err != nil {
		log.Fatalf("Error getting WireGuard keys: %v", err)
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error creating in-cluster config: %s", err)
	}
	nncCli := acn.NewNncClient(cfg)

	nnc, err := nncCli.GetNnc(nodeName)
	if err != nil {
		log.Fatalf("Error getting NodeNetworkConfig: %v", err)
	}

	if len(nnc.Status.NetworkContainers) == 0 {
		log.Fatalf("No network containers found in NodeNetworkConfig %v", nnc)
	}

	primaryIP := nnc.Status.NetworkContainers[0].PrimaryIP
	peerName := fmt.Sprintf("peer-%s", nodeName)

	peer := &v1alpha1.Peer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peerName,
			Namespace: "kube-system",
		},
		Spec: v1alpha1.PeerSpec{
			PublicKey:  publicKey,
			PodIPs:     []string{nodeIP},
			Endpoint:   nodeIP,
			AllowedIPs: []string{primaryIP},
		},
	}

	err = k8sClient.Create(context.Background(), peer)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			fmt.Println("Peer resource already exists, updating...")
			existingPeer := &v1alpha1.Peer{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: peerName, Namespace: "kube-system"}, existingPeer)
			if err != nil {
				log.Fatalf("Error getting existing Peer resource: %v", err)
			}

			existingPeer.Spec.PublicKey = publicKey
			existingPeer.Spec.PodIPs = []string{nodeIP}
			existingPeer.Spec.Endpoint = nodeIP

			err = k8sClient.Update(context.Background(), existingPeer)
			if err != nil {
				log.Fatalf("Error updating existing Peer resource: %v", err)
			}

			fmt.Println("Peer resource updated successfully.")
		} else {
			log.Fatalf("Error creating Peer resource: %v", err)
		}
	} else {
		fmt.Println("Peer resource created successfully.")
	}
}

func createK8sClient() (client.Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{Scheme: scheme})
}

func getNodeIP(k8sClient client.Client, nodeName string) (string, error) {
	node := &v1.Node{}
	err := k8sClient.Get(context.Background(), client.ObjectKey{Name: nodeName}, node)
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

func createConfigMap(k8sClient kubernetes.Interface, publicKey string) error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wg-publickey-config",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"publickey": publicKey,
		},
	}

	_, err := k8sClient.CoreV1().ConfigMaps("kube-system").Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			fmt.Println("ConfigMap already exists.")
			return nil
		}
		return err
	}
	return nil
}

func updateConfigMapWithPublicKey(clientset *kubernetes.Clientset, publicKey string) error {
	cm, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "wg-publickey-config", metav1.GetOptions{})
	if err != nil {
		return err
	}

	nodeName, err := os.Hostname()
	if err != nil {
		return err
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[nodeName] = publicKey

	_, err = clientset.CoreV1().ConfigMaps("kube-system").Update(context.Background(), cm, metav1.UpdateOptions{})
	return err
}

func getWireGuardKeyPair() (wgtypes.Key, wgtypes.Key, error) {
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return wgtypes.Key{}, wgtypes.Key{}, err
	}
	publicKey := privateKey.PublicKey()
	return privateKey, publicKey, nil
}

func getWireGuardKeys() (wgtypes.Key, string, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return wgtypes.Key{}, "", err
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return wgtypes.Key{}, "", err
	}

	_, publicKey, err := getWireGuardKeyPair()
	if err != nil {
		return wgtypes.Key{}, "", err
	}

	err = createConfigMap(k8sClient, publicKey.String())
	if err != nil {
		return wgtypes.Key{}, "", err
	}

	return wgtypes.Key{}, publicKey.String(), nil
}

func mustParseKey(s string) wgtypes.Key {
	k, err := wgtypes.ParseKey(s)
	if err != nil {
		panic(err)
	}
	return k
}
