// Copyright 2023 The KusionStack Authors
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

package main

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	gwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func init() {
	utilruntime.Must(rolloutv1alpha1.Install(clientgoscheme.Scheme))
	utilruntime.Must(operatingv1alpha1.Install(clientgoscheme.Scheme))
	utilruntime.Must(gatewayapiv1.Install(clientgoscheme.Scheme))
	utilruntime.Must(gwapiv1alpha2.Install(clientgoscheme.Scheme))
	//+kubebuilder:scaffold:scheme
}
