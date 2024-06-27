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
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PeerReconciler reconciles a Peer object
type PeerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=aks.azure.com,resources=peers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aks.azure.com,resources=peers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aks.azure.com,resources=peers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Peer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *PeerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//_ = log.FromContext(ctx)

	// TODO(user): your logic here
	log := ctrl.LoggerFrom(ctx)

	// Fetch the Peer instance
	var peer v1alpha1.Peer
	if err := r.Get(ctx, req.NamespacedName, &peer); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to get Peer")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure WireGuard setup
	if err := r.ensureWireGuardSetup(&peer); err != nil {
		log.Error(err, "Failed to ensure WireGuard setup")
		return ctrl.Result{}, err
	}

	//example logic to update the status of the peer
	// peer.Status.Conditions = append(peer.Status.Conditions, metav1.Condition{
	// 	Type:    "Ready",
	// 	Status:  metav1.ConditionTrue,
	// 	Reason:  "Reconciled",
	// 	Message: "Peer successfully reconciled",
	// })
	if err := r.Status().Update(ctx, &peer); err != nil {
		log.Error(err, "unable to update Peer status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ensureWireGuardSetup ensures the WireGuard setup for the Peer
func (r *PeerReconciler) ensureWireGuardSetup(peer *v1alpha1.Peer) error {
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
	// cfg := config.Config{
	// 	Interface: config.Interface{
	// 		PrivateKey:   peer.Spec.PrivateKey,
	// 		ListenPort:   peer.Spec.ListenPort,
	// 		ReplacePeers: true,
	// 	},
	// 	Peers: []config.Peer{
	// 		{
	// 			PublicKey:  peer.Spec.PublicKey,
	// 			Endpoint:   peer.Spec.Endpoint,
	// 			AllowedIPs: peer.Spec.PodIPs,
	// 		},
	// 	},
	// }
	cfg := wgtypes.Config{}
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

	log.Log.Info("WireGuard setup completed for Peer %s/%s", peer.Namespace, peer.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1alpha1.Peer{}).
		Complete(r)
}
