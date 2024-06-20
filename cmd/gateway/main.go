package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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
	log.Printf("wireguard device: %v", wgdev)
	// wgdev.PrivateKey = privateKey

	// route 10.244.0.0/16 traffic to wg
	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst: &net.IPNet{
			IP:   net.ParseIP("10.244.0.0"),
			Mask: net.CIDRMask(16, 32),
		},
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

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// List nodes every 2 seconds
	for {
		time.Sleep(2 * time.Second)

		nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
		if err != nil {
			log.Default().Printf("could not list nodes: %s\n", err)
			continue
		}

		for _, node := range nodes.Items {
			fmt.Printf("Node: %s\n", node.Name)
		}

	}
}
