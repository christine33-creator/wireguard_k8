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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PeerSpec defines the desired state of Peer
type PeerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Peer. Edit peer_types.go to remove/update
	// PublicKey is the WireGuard public key of the peer
	PrivateKey string   `json:"privateKey"`
	ListenPort int      `json:"listenPort"`
	PublicKey  string   `json:"publicKey"`
	Endpoint   string   `json:"endpoint"`
	PodIPs     []string `json:"podIPs"`
	MeshIP     string   `json:"meshIP,omitempty"`
	AllowedIPs []string `json:"allowedIPs,omitempty"`
}

// PeerStatus defines the observed state of Peer
type PeerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Peer is the Schema for the peers API
type Peer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PeerSpec   `json:"spec,omitempty"`
	Status PeerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PeerList contains a list of Peer
type PeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Peer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Peer{}, &PeerList{})
}
