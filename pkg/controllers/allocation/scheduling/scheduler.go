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

package scheduling

import (
	"context"
	"fmt"
	"time"

	"github.com/awslabs/karpenter/pkg/apis/provisioning/v1alpha4"
	"github.com/awslabs/karpenter/pkg/metrics"
	"github.com/awslabs/karpenter/pkg/scheduling"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var scheduleTimeHistogramVec = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: metrics.KarpenterNamespace,
		Subsystem: "allocation_controller",
		Name:      "scheduling_duration_seconds",
		Help:      "Duration of scheduling process in seconds. Broken down by provisioner and result.",
		Buckets:   metrics.DurationBuckets(),
	},
	[]string{metrics.ProvisionerLabel, metrics.ResultLabel},
)

func init() {
	crmetrics.Registry.MustRegister(scheduleTimeHistogramVec)
}

type Scheduler struct {
	KubeClient  client.Client
	Topology    *Topology
	Preferences *Preferences
}

type Schedule struct {
	*v1alpha4.Constraints
	// Pods is a set of pods that may schedule to the node; used for binpacking.
	Pods []*v1.Pod
	// Daemons are a set of daemons that will schedule to the node; used for overhead.
	Daemons []*v1.Pod
}

func NewScheduler(kubeClient client.Client) *Scheduler {
	return &Scheduler{
		KubeClient: kubeClient,
		Topology: &Topology{
			kubeClient: kubeClient,
		},
		Preferences: NewPreferences(),
	}
}

func (s *Scheduler) Solve(ctx context.Context, provisioner *v1alpha4.Provisioner, pods []*v1.Pod) ([]*Schedule, error) {
	startTime := time.Now()
	schedules, scheduleErr := s.solve(ctx, &provisioner.Spec.Constraints, pods)
	durationSeconds := time.Since(startTime).Seconds()

	result := "success"
	if scheduleErr != nil {
		result = "error"
	}

	newLabels := prometheus.Labels{
		metrics.ProvisionerLabel: provisioner.ObjectMeta.Name,
		metrics.ResultLabel:      result,
	}
	observer, promErr := scheduleTimeHistogramVec.GetMetricWith(newLabels)
	if promErr != nil {
		logging.FromContext(ctx).Warnf(
			"Failed to record scheduling duration metric [labels=%s, duration=%f]: error=%s",
			newLabels,
			durationSeconds,
			promErr.Error(),
		)
	} else {
		observer.Observe(durationSeconds)
	}

	return schedules, scheduleErr
}

func (s *Scheduler) solve(ctx context.Context, constraints *v1alpha4.Constraints, pods []*v1.Pod) ([]*Schedule, error) {
	// Apply runtime constraints
	constraints = constraints.DeepCopy()
	if err := constraints.Constrain(ctx); err != nil {
		return nil, fmt.Errorf("applying constraints, %w", err)
	}
	// Relax preferences if pods have previously failed to schedule.
	s.Preferences.Relax(ctx, pods)
	// Inject temporarily adds specific NodeSelectors to pods, which are then
	// used by scheduling logic. This isn't strictly necessary, but is a useful
	// trick to avoid passing topology decisions through the scheduling code. It
	// lets us to treat TopologySpreadConstraints as just-in-time NodeSelectors.
	if err := s.Topology.Inject(ctx, constraints, pods); err != nil {
		return nil, fmt.Errorf("injecting topology, %w", err)
	}
	// Separate pods into schedules of isomorphic scheduling constraints.
	schedules, err := s.getSchedules(ctx, constraints, pods)
	if err != nil {
		return nil, fmt.Errorf("getting schedules, %w", err)
	}
	// Remove labels injected by TopologySpreadConstraints.
	for _, schedule := range schedules {
		delete(schedule.Labels, v1.LabelHostname)
	}
	return schedules, nil
}

// getSchedules separates pods into a set of schedules. All pods in each group
// contain isomorphic scheduling constraints and can be deployed together on the
// same node, or multiple similar nodes if the pods exceed one node's capacity.
func (s *Scheduler) getSchedules(ctx context.Context, v1alpha4constraints *v1alpha4.Constraints, pods []*v1.Pod) ([]*Schedule, error) {
	// schedule uniqueness is tracked by hash(Constraints)
	schedules := map[uint64]*Schedule{}
	for _, pod := range pods {
		constraints, err := NewConstraints(ctx, v1alpha4constraints, pod)
		if err != nil {
			logging.FromContext(ctx).Debugf("Ignored pod %s/%s due to invalid constraints, %s", pod.Name, pod.Namespace, err.Error())
			continue
		}
		key, err := hashstructure.Hash(constraints, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
		if err != nil {
			return nil, fmt.Errorf("hashing constraints, %w", err)
		}
		// Create new schedule if one doesn't exist
		if _, ok := schedules[key]; !ok {
			// Uses a theoretical node object to compute schedulablility of daemonset overhead.
			daemons, err := s.getDaemons(ctx, constraints)
			if err != nil {
				return nil, fmt.Errorf("computing node overhead, %w", err)
			}
			schedules[key] = &Schedule{
				Constraints: constraints,
				Pods:        []*v1.Pod{},
				Daemons:     daemons,
			}
		}
		// Append pod to schedule, guaranteed to exist
		schedules[key].Pods = append(schedules[key].Pods, pod)
	}

	result := []*Schedule{}
	for _, schedule := range schedules {
		result = append(result, schedule)
	}
	return result, nil
}

func (s *Scheduler) getDaemons(ctx context.Context, constraints *v1alpha4.Constraints) ([]*v1.Pod, error) {
	// 1. Get DaemonSets
	daemonSetList := &appsv1.DaemonSetList{}
	if err := s.KubeClient.List(ctx, daemonSetList); err != nil {
		return nil, fmt.Errorf("listing daemonsets, %w", err)
	}

	// 2. filter DaemonSets to include those that will schedule on this node
	pods := []*v1.Pod{}
	for _, daemonSet := range daemonSetList.Items {
		pod := &v1.Pod{Spec: daemonSet.Spec.Template.Spec}
		if DaemonWillSchedule(constraints, pod) {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

// DaemonWillSchedule returns true if the pod can schedule to the node
func DaemonWillSchedule(constraints *v1alpha4.Constraints, pod *v1.Pod) bool {
	// Tolerate Taints
	if err := scheduling.Taints(constraints.Taints).Tolerates(pod); err != nil {
		return false
	}
	// Match Node Selector labels
	if !labels.SelectorFromSet(pod.Spec.NodeSelector).Matches(labels.Set(constraints.Labels)) {
		return false
	}
	// TODO support node affinity for daemonset
	return true
}
