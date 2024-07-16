package acn

import (
	"context"

	acnv1alpha "github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	_ = acnv1alpha.AddToScheme(scheme)
}

type NncClient struct {
	client client.Client
}

func NewNncClient(cfg *rest.Config) *NncClient {
	c, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		panic(err)
	}
	return &NncClient{client: c}
}

func (n *NncClient) GetNnc(nodeName string) (*acnv1alpha.NodeNetworkConfig, error) {
	nnc := &acnv1alpha.NodeNetworkConfig{}
	err := n.client.Get(context.TODO(), client.ObjectKey{
		Name:      nodeName,
		Namespace: metav1.NamespaceSystem,
	}, nnc)
	if err != nil {
		return nil, err
	}
	return nnc, nil
}
