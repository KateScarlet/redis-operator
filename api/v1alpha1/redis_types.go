/*
Copyright 2025.

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

type ResourceRequirements struct {
	// +optional
	Requests map[string]string `json:"requests,omitempty"`
	// +optional
	Limits map[string]string `json:"limits,omitempty"`
}
type SideCarContainer struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	Image string `json:"image,omitempty"`
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty"`
}

type RedisVolume struct {
	// +optional
	StorageClass string `json:"storageclass,omitempty"`
	// +optional
	Size string `json:"size,omitempty"`
}

type RedisSentinel struct {
	Enabled   bool                 `json:"enabled,omitempty"`
	Replicas  int32                `json:"replicas,omitempty"`
	Image     string               `json:"image,omitempty"`
	Resources ResourceRequirements `json:"resources,omitempty"`
}

type RedisAOFConfig struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	Fsync string `json:"fsync,omitempty"`
}

// RedisSpec defines the desired state of Redis.
type RedisSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Image     string               `json:"image,omitempty"`
	Resources ResourceRequirements `json:"resources,omitempty"`
	Exporter  SideCarContainer     `json:"exporter,omitempty"`
	Password  string               `json:"password,omitempty"`
	Replicas  int32                `json:"replicas,omitempty"`
	Volume    RedisVolume          `json:"volume,omitempty"`
	Sentinel  RedisSentinel        `json:"sentinel,omitempty"`
	AOF       RedisAOFConfig       `json:"AOF,omitempty"`
}

// RedisStatus defines the observed state of Redis.
type RedisStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Redis is the Schema for the redis API.
type Redis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSpec   `json:"spec,omitempty"`
	Status RedisStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedisList contains a list of Redis.
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Redis{}, &RedisList{})
}
