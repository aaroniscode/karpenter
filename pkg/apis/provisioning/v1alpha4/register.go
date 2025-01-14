/*
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

package v1alpha4

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
)

var (
	ArchitectureAmd64    = "amd64"
	ArchitectureArm64    = "arm64"
	OperatingSystemLinux = "linux"

	ProvisionerNameLabelKey         = SchemeGroupVersion.Group + "/provisioner-name"
	NotReadyTaintKey                = SchemeGroupVersion.Group + "/not-ready"
	DoNotEvictPodAnnotationKey      = SchemeGroupVersion.Group + "/do-not-evict"
	EmptinessTimestampAnnotationKey = SchemeGroupVersion.Group + "/emptiness-timestamp"
	TerminationFinalizer            = SchemeGroupVersion.Group + "/termination"
	DefaultProvisioner              = types.NamespacedName{Name: "default"}
)

var (
	// RestrictedLabels are injected by Cloud Providers
	RestrictedLabels = []string{
		// Use strongly typed fields instead
		v1.LabelArchStable,
		v1.LabelOSStable,
		v1.LabelTopologyZone,
		v1.LabelInstanceTypeStable,
		// Used internally by provisioning logic
		EmptinessTimestampAnnotationKey,
		v1.LabelHostname,
	}
	// WellKnownLabels supported by karpenter and their allowable values
	WellKnownLabels = map[string][]string{
		v1.LabelArchStable:         {},
		v1.LabelOSStable:           {},
		v1.LabelTopologyZone:       {},
		v1.LabelInstanceTypeStable: {},
	}
	DefaultHook   = func(ctx context.Context, constraints *Constraints) {}
	ValidateHook  = func(ctx context.Context, constraints *Constraints) *apis.FieldError { return nil }
	ConstrainHook = func(ctx context.Context, constraints *Constraints, pods ...*v1.Pod) error { return nil }
)

var (
	Group              = "karpenter.sh"
	ExtensionsGroup    = "extensions." + Group
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: "v1alpha4"}
	SchemeBuilder      = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(SchemeGroupVersion,
			&Provisioner{},
			&ProvisionerList{},
		)
		metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
		return nil
	})
)

const (
	// Active is a condition implemented by all resources. It indicates that the
	// controller is able to take actions: it's correctly configured, can make
	// necessary API calls, and isn't disabled.
	Active apis.ConditionType = "Active"
)
