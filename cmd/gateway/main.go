package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/t-chdossa_microsoft/aks-mesh/api/v1alpha1"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const gatewayInfName = "wgg" // "wireguardgateway"

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

// port 51820 (gateway)
// port 51821 (client)

type WireGuard struct {
	Attributes *netlink.LinkAttrs
}

// Attrs implements netlink.Link.
func (w *WireGuard) Attrs() *netlink.LinkAttrs {
	return w.Attributes
}

// Type implements netlink.Link.
func (w *WireGuard) Type() string {
	return "wireguard"
}

var _ netlink.Link = &WireGuard{}

func main() {
	var (
		podCIDR         string
		gatewayEndpoint string
		nodeName        string
	)
	flag.StringVar(&podCIDR, "pod-cidr", "", "Overall pod cidr of the cluster")
	flag.StringVar(&nodeName, "node-name", "", "Name of the node")
	flag.StringVar(&gatewayEndpoint, "gateway-endpoint", "", "Endpoint of the gateway")
	flag.Parse()
	if podCIDR == "" {
		panic("pod-cidr flag is required")
	}

	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// initialize wireguard interface
	// create a new wireguard interface
	la := netlink.NewLinkAttrs()
	la.Name = gatewayInfName
	l := &WireGuard{Attributes: &la}

	err := netlink.LinkAdd(l)
	if err != nil && !errors.Is(err, os.ErrExist) {
		panic(err)
	}

	link, err := netlink.LinkByName(gatewayInfName)
	if err != nil {
		log.Fatal(err)
	}

	addr := netlink.Addr{
		IPNet: &net.IPNet{
			// 100.255.254.0/19
			IP:   net.ParseIP("100.255.254.4"),
			Mask: net.CIDRMask(19, 32),
		},
	}
	err = netlink.AddrAdd(link, &addr)
	if err != nil && !errors.Is(err, os.ErrExist) {
		log.Fatal(err)
	}

	err = netlink.LinkSetUp(link)
	if err != nil {
		log.Fatalf("failed to bring up link: %s", err)
	}

	log.Println("wireguard link created and initialized")

	cli, _ := wgctrl.New()
	defer cli.Close()

	wgdev, err := cli.Device(gatewayInfName)
	if err != nil {
		log.Fatalf("failed to get wireguard device: %s", err)
	}

	// todo read secret containing the wireguard config for the gateways
	// generate a new wireguard private and public key
	k, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		panic(err)
	}

	log.Printf("wireguard device: %v", wgdev)
	err = cli.ConfigureDevice(wgdev.Name, wgtypes.Config{
		PrivateKey: &k,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to configure wireguard device: %s", err))
	}

	// route 10.244.0.0/16 traffic to wg
	_, podIPNet, err := net.ParseCIDR(podCIDR)
	if err != nil {
		panic(fmt.Sprintf("failed to parse podCIDR: %s", err))
	}

	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       podIPNet,
		Scope:     netlink.SCOPE_LINK,
	}
	err = netlink.RouteAdd(route)
	if err != nil {
		log.Printf("failed to add route: %s", err)
	}

	// Get the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	c, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create client: %v", err))
	}

	gw := &v1alpha1.Gateway{}
	err = c.Get(context.Background(), client.ObjectKey{
		Namespace: v1.NamespaceSystem,
		Name:      nodeName,
	}, gw)

	if err != nil && !apierrors.IsNotFound(err) {
		panic(fmt.Sprintf("failed to get gateway: %v", err))
	}

	if apierrors.IsNotFound(err) {
		gw = &v1alpha1.Gateway{
			ObjectMeta: v1.ObjectMeta{
				Name:      nodeName,
				Namespace: v1.NamespaceSystem,
			},
			Spec: v1alpha1.GatewaySpec{
				PublicKey: k.PublicKey().String(),
				Endpoint:  gatewayEndpoint,
			},
		}
		err = c.Create(context.Background(), gw)
		if err != nil {
			panic(fmt.Sprintf("failed to create gateway: %v", err))
		}
	} else {
		// update if publickey or endpoint has changed
		if gw.Spec.PublicKey != k.PublicKey().String() || gw.Spec.Endpoint != gatewayEndpoint {
			gw.Spec.PublicKey = k.PublicKey().String()
			gw.Spec.Endpoint = gatewayEndpoint
			err = c.Update(context.Background(), gw)
			if err != nil {
				panic(fmt.Sprintf("failed to update gateway: %v", err))
			}
		}
	}

	// List nodes every 2 seconds
	peerCache := make(map[string]v1alpha1.Peer)
	wgdev, err = cli.Device(gatewayInfName)
	if err != nil {
		log.Fatalf("failed to get wireguard device: %s", err)
	}

	for _, p := range wgdev.Peers {
		peerCache[p.PublicKey.String()] = v1alpha1.Peer{}
	}

	for {
		select {
		case sig := <-sigChan:
			log.Printf("received signal: %s, performing cleanup", sig)
			cleanup(nodeName)
			return
		default:
			time.Sleep(2 * time.Second)
		}

		peers := &v1alpha1.PeerList{}
		err = c.List(context.Background(), peers)
		if err != nil {
			log.Default().Printf("could not list peers: %s\n", err)
			continue
		}

		for _, peer := range peers.Items {
			if _, ok := peerCache[peer.Spec.PublicKey]; !ok {
				// add peer to wireguard device
				cfg := wgtypes.PeerConfig{
					PublicKey: mustParseKey(peer.Spec.PublicKey),
					Endpoint:  &net.UDPAddr{IP: net.ParseIP(peer.Spec.Endpoint), Port: 51821},
					AllowedIPs: []net.IPNet{
						{
							IP:   net.ParseIP(peer.Spec.MeshIP),
							Mask: net.CIDRMask(32, 32),
						},
					},
				}

				for _, allowedIP := range peer.Spec.AllowedIPs {
					_, ipNet, err := net.ParseCIDR(allowedIP)
					if err != nil {
						log.Printf("failed to parse allowed ip: %s", err)
						continue
					}
					cfg.AllowedIPs = append(cfg.AllowedIPs, *ipNet)
				}

				err = cli.ConfigureDevice(wgdev.Name, wgtypes.Config{
					Peers:        []wgtypes.PeerConfig{cfg},
					ReplacePeers: false,
				})
				if err != nil {
					log.Printf("failed to add peer to wireguard device: %s", err)
					continue
				}
				peerCache[peer.Spec.PublicKey] = peer
			}
		}
	}
}

func cleanup(gatewayName string) {
	// delete the wgg interface if it exists
	link, err := netlink.LinkByName(gatewayInfName)
	if err == nil {
		err = netlink.LinkDel(link)
		if err != nil {
			log.Printf("failed to delete link: %s", err)
		}
	}

	// delete the gateway resource
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	c, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create client: %v", err))
	}
	err = c.Get(context.Background(), client.ObjectKey{
		Namespace: v1.NamespaceSystem,
		Name:      gatewayName,
	}, &v1alpha1.Gateway{})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Printf("failed to get gateway: %s", err)
		return
	}
	if apierrors.IsNotFound(err) {
		log.Printf("gateway resource not found, nothing to do")
		return
	}

	err = c.Delete(context.Background(), &v1alpha1.Gateway{
		ObjectMeta: v1.ObjectMeta{
			Name:      gatewayName,
			Namespace: v1.NamespaceSystem,
		},
	})
	if err != nil {
		log.Printf("failed to delete gateway: %s", err)
	}
}

func mustParseKey(s string) wgtypes.Key {
	k, err := wgtypes.ParseKey(s)
	if err != nil {
		panic(err)
	}
	return k
}
