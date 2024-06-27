package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
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
	var podCIDR string
	flag.StringVar(&podCIDR, "pod-cidr", "", "Overall pod cidr of the cluster")
	flag.Parse()
	if podCIDR == "" {
		panic("pod-cidr flag is required")
	}

	// initialize wireguard interface
	// create a new wireguard interface
	la := netlink.NewLinkAttrs()
	la.Name = "wg0"
	l := &WireGuard{Attributes: &la}

	err := netlink.LinkAdd(l)
	if err != nil && !errors.Is(err, os.ErrExist) {
		panic(err)
	}

	link, err := netlink.LinkByName("wg0")
	if err != nil {
		log.Fatal(err)
	}

	podIP := os.Getenv("POD_IP")
	if podIP == "" {
		panic("POD_IP env var is required")
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

	wgdev, err := cli.Device("wg0")
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
	gatewayName := "gateway-" + podIP
	gw := &v1alpha1.Gateway{}
	err = c.Get(context.Background(), client.ObjectKey{
		Namespace: "kube-system",
		Name:      gatewayName,
	}, gw)

	if err != nil && !apierrors.IsNotFound(err) {
		panic(fmt.Sprintf("failed to get gateway: %v", err))
	}

	if apierrors.IsNotFound(err) {
		gw = &v1alpha1.Gateway{
			ObjectMeta: v1.ObjectMeta{
				Name:      gatewayName,
				Namespace: "kube-system",
			},
			Spec: v1alpha1.GatewaySpec{
				PublicKey: k.PublicKey().String(),
				Endpoint:  podIP,
			},
		}
		err = c.Create(context.Background(), gw)
		if err != nil {
			panic(fmt.Sprintf("failed to create gateway: %v", err))
		}
	} else {
		// update if publickey or endpoint has changed
		if gw.Spec.PublicKey != k.PublicKey().String() || gw.Spec.Endpoint != podIP {
			gw.Spec.PublicKey = k.PublicKey().String()
			gw.Spec.Endpoint = podIP
			err = c.Update(context.Background(), gw)
			if err != nil {
				panic(fmt.Sprintf("failed to update gateway: %v", err))
			}
		}
	}

	// List nodes every 2 seconds
	peerCache := make(map[string]v1alpha1.Peer)
	wgdev, err = cli.Device("wg0")
	if err != nil {
		log.Fatalf("failed to get wireguard device: %s", err)
	}

	for _, p := range wgdev.Peers {
		peerCache[p.PublicKey.String()] = v1alpha1.Peer{}
	}

	for {
		time.Sleep(2 * time.Second)

		peers := &v1alpha1.PeerList{}
		err = c.List(context.Background(), peers)
		if err != nil {
			log.Default().Printf("could not list peers: %s\n", err)
			continue
		}

		for _, peer := range peers.Items {
			if _, ok := peerCache[peer.Spec.PublicKey]; !ok {
				peerCache[peer.Spec.PublicKey] = peer
				// add peer to wireguard device
				cfg := wgtypes.PeerConfig{
					PublicKey: mustParseKey(peer.Spec.PublicKey),
					Endpoint:  &net.UDPAddr{IP: net.ParseIP(peer.Spec.Endpoint), Port: 51820},
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
				}
			}
		}
	}
}

func mustParseKey(s string) wgtypes.Key {
	k, err := wgtypes.ParseKey(s)
	if err != nil {
		panic(err)
	}
	return k
}
