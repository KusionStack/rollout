// Copyright 2026 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rollout

import (
	"errors"
	"strings"
	"sync"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
)

var testCollaSetGVK = schema.GroupVersionKind{
	Group:   "apps.kusionstack.io",
	Version: "v1alpha1",
	Kind:    "CollaSet",
}

type targetedDiscovery struct {
	discovery.DiscoveryInterface
	mu                 sync.Mutex
	groups             *metav1.APIGroupList
	resources          map[string]*metav1.APIResourceList
	errors             map[string]error
	targetCalls        []string
	fullDiscoveryCalls int
}

func (d *targetedDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.targetCalls = append(d.targetCalls, groupVersion)
	if err := d.errors[groupVersion]; err != nil {
		return nil, err
	}
	return d.resources[groupVersion], nil
}

func (d *targetedDiscovery) ServerGroups() (*metav1.APIGroupList, error) {
	return d.groups, nil
}

func (d *targetedDiscovery) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	d.fullDiscoveryCalls++
	return nil, nil, errors.New("unrelated metrics/v1alpha1 discovery failed")
}

func resourceList(groupVersion string, kinds ...string) *metav1.APIResourceList {
	resources := make([]metav1.APIResource, 0, len(kinds))
	for _, kind := range kinds {
		resources = append(resources, metav1.APIResource{Kind: kind})
	}
	return &metav1.APIResourceList{GroupVersion: groupVersion, APIResources: resources}
}

func TestSingleClusterDiscoveryTargetsRequestedGVK(t *testing.T) {
	groupVersion := testCollaSetGVK.GroupVersion().String()
	client := &targetedDiscovery{
		resources: map[string]*metav1.APIResourceList{
			groupVersion: resourceList(groupVersion, "CollaSet"),
		},
		errors: map[string]error{
			"metrics/v1alpha1": errors.New("got empty response"),
		},
	}

	supported, msg, err := (&singleClusterDiscovery{client: client}).IsSupported(testCollaSetGVK)
	if err != nil {
		t.Fatalf("IsSupported() error = %v", err)
	}
	if !supported || msg != "" {
		t.Fatalf("IsSupported() = (%v, %q), want (true, empty)", supported, msg)
	}
	if len(client.targetCalls) != 1 || client.targetCalls[0] != groupVersion {
		t.Fatalf("target discovery calls = %v, want [%s]", client.targetCalls, groupVersion)
	}
	if client.fullDiscoveryCalls != 0 {
		t.Fatalf("full discovery calls = %d, want 0", client.fullDiscoveryCalls)
	}
}

func TestSingleClusterDiscoveryWithMemoryCacheIgnoresUnrelatedGroupFailure(t *testing.T) {
	groupVersion := testCollaSetGVK.GroupVersion().String()
	client := memory.NewMemCacheClient(&targetedDiscovery{
		groups: &metav1.APIGroupList{Groups: []metav1.APIGroup{
			{
				Name: testCollaSetGVK.Group,
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: groupVersion, Version: testCollaSetGVK.Version},
				},
			},
			{
				Name: "metrics",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "metrics/v1alpha1", Version: "v1alpha1"},
				},
			},
		}},
		resources: map[string]*metav1.APIResourceList{
			groupVersion: resourceList(groupVersion, "CollaSet"),
		},
		errors: map[string]error{
			"metrics/v1alpha1": errors.New("got empty response"),
		},
	})

	supported, msg, err := (&singleClusterDiscovery{client: client}).IsSupported(testCollaSetGVK)
	if err != nil {
		t.Fatalf("IsSupported() error = %v", err)
	}
	if !supported || msg != "" {
		t.Fatalf("IsSupported() = (%v, %q), want (true, empty)", supported, msg)
	}
}

func TestSingleClusterDiscoveryUnsupportedTarget(t *testing.T) {
	groupVersion := testCollaSetGVK.GroupVersion().String()
	tests := []struct {
		name      string
		resources map[string]*metav1.APIResourceList
		err       error
	}{
		{
			name: "kind missing",
			resources: map[string]*metav1.APIResourceList{
				groupVersion: resourceList(groupVersion, "StatefulSet"),
			},
		},
		{
			name: "group version mismatch",
			resources: map[string]*metav1.APIResourceList{
				groupVersion: resourceList("apps.kusionstack.io/v1beta1", "CollaSet"),
			},
		},
		{name: "cache miss", err: memory.ErrCacheNotFound},
		{
			name: "not found",
			err: apierrors.NewNotFound(schema.GroupResource{
				Group:    testCollaSetGVK.Group,
				Resource: "collasets",
			}, ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &targetedDiscovery{
				resources: tt.resources,
				errors:    map[string]error{groupVersion: tt.err},
			}
			supported, msg, err := (&singleClusterDiscovery{client: client}).IsSupported(testCollaSetGVK)
			if err != nil {
				t.Fatalf("IsSupported() error = %v", err)
			}
			if supported {
				t.Fatal("IsSupported() = true, want false")
			}
			if !strings.Contains(msg, testCollaSetGVK.String()) {
				t.Fatalf("IsSupported() message = %q, want target GVK", msg)
			}
		})
	}
}

func TestSingleClusterDiscoveryFailsClosedForTargetErrors(t *testing.T) {
	groupVersion := testCollaSetGVK.GroupVersion().String()
	tests := []struct {
		name      string
		resources map[string]*metav1.APIResourceList
		err       error
	}{
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Group: testCollaSetGVK.Group, Resource: "collasets"}, "", errors.New("forbidden"))},
		{name: "server error", err: apierrors.NewInternalError(errors.New("server error"))},
		{name: "nil response", resources: map[string]*metav1.APIResourceList{groupVersion: nil}},
		{name: "empty response", resources: map[string]*metav1.APIResourceList{groupVersion: resourceList(groupVersion)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &targetedDiscovery{
				resources: tt.resources,
				errors:    map[string]error{groupVersion: tt.err},
			}
			supported, _, err := (&singleClusterDiscovery{client: client}).IsSupported(testCollaSetGVK)
			if err == nil {
				t.Fatal("IsSupported() error = nil, want target discovery error")
			}
			if supported {
				t.Fatal("IsSupported() = true, want false")
			}
		})
	}
}

func TestMultiClusterDiscoveryRequiresEveryMember(t *testing.T) {
	groupVersion := testCollaSetGVK.GroupVersion().String()
	newClient := func(kinds ...string) *targetedDiscovery {
		return &targetedDiscovery{
			resources: map[string]*metav1.APIResourceList{
				groupVersion: resourceList(groupVersion, kinds...),
			},
			errors: map[string]error{
				"metrics/v1alpha1": errors.New("got empty response"),
			},
		}
	}

	t.Run("all members support target", func(t *testing.T) {
		memberA := newClient("CollaSet")
		memberB := newClient("CollaSet")
		discoveryClient := &multiclusterDiscovery{clients: map[string]discovery.DiscoveryInterface{
			"member-a": memberA,
			"member-b": memberB,
		}}

		supported, msg, err := discoveryClient.IsSupported(testCollaSetGVK)
		if err != nil {
			t.Fatalf("IsSupported() error = %v", err)
		}
		if !supported || msg != "" {
			t.Fatalf("IsSupported() = (%v, %q), want (true, empty)", supported, msg)
		}
		for name, client := range map[string]*targetedDiscovery{"member-a": memberA, "member-b": memberB} {
			if len(client.targetCalls) != 1 || client.targetCalls[0] != groupVersion {
				t.Fatalf("%s target discovery calls = %v, want [%s]", name, client.targetCalls, groupVersion)
			}
			if client.fullDiscoveryCalls != 0 {
				t.Fatalf("%s full discovery calls = %d, want 0", name, client.fullDiscoveryCalls)
			}
		}
	})

	t.Run("one member misses target kind", func(t *testing.T) {
		discoveryClient := &multiclusterDiscovery{clients: map[string]discovery.DiscoveryInterface{
			"member-a": newClient("CollaSet"),
			"member-b": newClient("StatefulSet"),
		}}

		supported, msg, err := discoveryClient.IsSupported(testCollaSetGVK)
		if err != nil {
			t.Fatalf("IsSupported() error = %v", err)
		}
		if supported {
			t.Fatal("IsSupported() = true, want false")
		}
		if !strings.Contains(msg, "member-b") || strings.Contains(msg, "member-a") {
			t.Fatalf("IsSupported() message = %q, want only unsupported member-b", msg)
		}
	})

	t.Run("one member target discovery fails", func(t *testing.T) {
		failedClient := newClient("CollaSet")
		failedClient.errors[groupVersion] = errors.New("target discovery failed")
		discoveryClient := &multiclusterDiscovery{clients: map[string]discovery.DiscoveryInterface{
			"member-a": newClient("CollaSet"),
			"member-b": failedClient,
		}}

		supported, _, err := discoveryClient.IsSupported(testCollaSetGVK)
		if err == nil || !strings.Contains(err.Error(), "member-b") {
			t.Fatalf("IsSupported() error = %v, want member-b target discovery error", err)
		}
		if supported {
			t.Fatal("IsSupported() = true, want false")
		}
	})

	t.Run("zero members preserves existing behavior", func(t *testing.T) {
		supported, msg, err := (&multiclusterDiscovery{}).IsSupported(testCollaSetGVK)
		if err != nil {
			t.Fatalf("IsSupported() error = %v", err)
		}
		if !supported || msg != "" {
			t.Fatalf("IsSupported() = (%v, %q), want (true, empty)", supported, msg)
		}
	})
}
