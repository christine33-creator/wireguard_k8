/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
        "errors"
        "net"
        "os"

        v1alpha1 "github.com/t-chdossa_microsoft/aks-mesh/api/v1alpha1"
        "github.com/vishvananda/netlink"
        "golang.zx2c4.com/wireguard/wgctrl"
        "k8s.io/apimachinery/pkg/runtime"
        ctrl "sigs.k8s.io/controller-runtime"
        "sigs.k8s.io/controller-runtime/pkg/client"
        ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
	v1alpha1 "github.com/t-chdossa_microsoft/aks-mesh/api/v1alpha1"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=aks.azure.com,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aks.azure.com,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aks.azure.com,resources=gateways/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//_ = log.FromContext(ctx)

	// TODO(user): your logic here
	log := ctrlLog.FromContext(ctx)

	// Fetch the Gateway instance
	var gateway v1alpha1.Gateway
	if err := r.Get(ctx, req.NamespacedName, &gateway); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to get Gateway")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure WireGuard setup
	if err := r.ensureWireGuardSetup(&gateway); err != nil {
		log.Error(err, "Failed to ensure WireGuard setup")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) ensureWireGuardSetup(gateway *v1alpha1.Gateway) error {
	la := netlink.NewLinkAttrs()
	la.Name = "wg0"
	link := &netlink.GenericLink{LinkAttrs: la, LinkType: "wireguard"}

	// Ensure the WireGuard interface exists
	if err := netlink.LinkAdd(link); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	// Get the WireGuard interface
	wgLink, err := netlink.LinkByName("wg0")
	if err != nil {
		return err
	}

	// Bring the interface up
	if err := netlink.LinkSetUp(wgLink); err != nil {
		return err
	}

	// Configure the WireGuard device
	cli, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer cli.Close()

	// Create a WireGuard configuration
	cfg := config.Config{
		Interface: config.Interface{
			PrivateKey:   gateway.Spec.PrivateKey,
			ListenPort:   gateway.Spec.ListenPort,
			ReplacePeers: true,
		},
		Peers: []config.Peer{
			{
				PublicKey: gateway.Spec.PublicKey,
				Endpoint:  gateway.Spec.Endpoint,
			},
		},
	}

	if err := cli.ConfigureDevice("wg0", cfg); err != nil {
		return err
	}

	// Add a route for the WireGuard network
	route := &netlink.Route{
		LinkIndex: wgLink.Attrs().Index,
		Dst: &net.IPNet{
			IP:   net.ParseIP("10.244.0.0"),
			Mask: net.CIDRMask(16, 32),
		},
	}
	if err := netlink.RouteAdd(route); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	log.Printf("WireGuard setup completed for Gateway %s/%s", gateway.Namespace, gateway.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1alpha1.Gateway{}).
		Complete(r)
}
