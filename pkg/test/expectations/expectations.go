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

package expecations

import (
	"context"
	"fmt"
	"time"

	//nolint:revive,stylecheck
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/awslabs/karpenter/pkg/apis/provisioning/v1alpha4"
)

const (
	ReconcilerPropagationTime = 10 * time.Second
	RequestInterval           = 1 * time.Second
)

func ExpectPodExists(c client.Client, name string, namespace string) *v1.Pod {
	pod := &v1.Pod{}
	Expect(c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, pod)).To(Succeed())
	return pod
}

func ExpectNodeExists(c client.Client, name string) *v1.Node {
	node := &v1.Node{}
	Expect(c.Get(context.Background(), client.ObjectKey{Name: name}, node)).To(Succeed())
	return node
}

func ExpectNotFound(c client.Client, objects ...client.Object) {
	for _, object := range objects {
		Eventually(func() bool {
			return errors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object))
		}, ReconcilerPropagationTime, RequestInterval).Should(BeTrue(), func() string {
			return fmt.Sprintf("expected %s to be deleted, but it still exists", object.GetSelfLink())
		})
	}
}

func ExpectCreated(c client.Client, objects ...client.Object) {
	for _, object := range objects {
		Expect(c.Create(context.Background(), object)).To(Succeed())
	}
}

func ExpectCreatedWithStatus(c client.Client, objects ...client.Object) {
	for _, object := range objects {
		// Preserve a copy of the status, which is overriden by create
		status := object.DeepCopyObject().(client.Object)
		ExpectCreated(c, object)
		Expect(c.Status().Update(context.Background(), status)).To(Succeed())
	}
}

func ExpectDeleted(c client.Client, objects ...client.Object) {
	for _, object := range objects {
		persisted := object.DeepCopyObject()
		object.SetFinalizers([]string{})
		Expect(c.Patch(context.Background(), object, client.MergeFrom(persisted))).To(Succeed())
		if err := c.Delete(context.Background(), object, &client.DeleteOptions{GracePeriodSeconds: ptr.Int64(0)}); !errors.IsNotFound(err) {
			Expect(err).To(BeNil())
		}
	}
	for _, object := range objects {
		ExpectNotFound(c, object)
	}
}

func ExpectCleanedUp(c client.Client) {
	ctx := context.Background()
	pdbs := v1beta1.PodDisruptionBudgetList{}
	Expect(c.List(ctx, &pdbs)).To(Succeed())
	for i := range pdbs.Items {
		ExpectDeleted(c, &pdbs.Items[i])
	}
	pods := v1.PodList{}
	Expect(c.List(ctx, &pods)).To(Succeed())
	for i := range pods.Items {
		ExpectDeleted(c, &pods.Items[i])
	}
	nodes := v1.NodeList{}
	Expect(c.List(ctx, &nodes)).To(Succeed())
	for i := range nodes.Items {
		ExpectDeleted(c, &nodes.Items[i])
	}
	provisioners := v1alpha4.ProvisionerList{}
	Expect(c.List(ctx, &provisioners)).To(Succeed())
	for i := range provisioners.Items {
		ExpectDeleted(c, &provisioners.Items[i])
	}
}

func ExpectProvisioningSucceeded(ctx context.Context, c client.Client, reconciler reconcile.Reconciler, provisioner *v1alpha4.Provisioner, pods ...*v1.Pod) []*v1.Pod {
	for _, pod := range pods {
		ExpectCreatedWithStatus(c, pod)
	}
	ExpectReconcileSucceeded(ctx, reconciler, client.ObjectKeyFromObject(provisioner))
	result := []*v1.Pod{}
	for _, pod := range pods {
		result = append(result, ExpectPodExists(c, pod.GetName(), pod.GetNamespace()))
	}
	return result
}

func ExpectReconcileSucceeded(ctx context.Context, reconciler reconcile.Reconciler, key client.ObjectKey) {
	_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
	Expect(err).ToNot(HaveOccurred())
}
