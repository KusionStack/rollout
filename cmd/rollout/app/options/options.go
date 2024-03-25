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

package options

import (
	"fmt"

	"github.com/spf13/pflag"
	cliflag "k8s.io/component-base/cli/flag"
	"kusionstack.io/kube-utils/controller/initializer"
	"kusionstack.io/kube-utils/multicluster/clusterprovider"

	"kusionstack.io/rollout/pkg/features"
)

type Options struct {
	ClusterConfigProvider clusterprovider.ClusterConfigProvider
	Controller            *ControllerOptions
	Log                   *LogOptions
	Serving               *ServingOptions
}

func NewOptions() *Options {
	return &Options{
		Controller: NewControllerOptions(),
		Log:        NewLogOptions(),
		Serving:    NewServingOptions(),
	}
}

func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.Controller.Validate()...)
	errs = append(errs, o.Serving.Validate()...)
	errs = append(errs, o.Log.Validate()...)

	return errs
}

func (o *Options) Flags(initializers ...initializer.Interface) *cliflag.NamedFlagSets {
	fss := &cliflag.NamedFlagSets{}
	fs := fss.FlagSet("options")

	// add feature gate flags
	features.DefaultMutableFeatureGate.AddFlag(fs)

	// bind initializer flags
	for _, in := range initializers {
		in.BindFlag(fs)
	}

	// bind controller flags
	o.Controller.BindFlags(fs)
	// bind serving flags
	o.Serving.BindFlags(fss.FlagSet("serving"))
	// bind log flags
	o.Log.BindFlags(fss.FlagSet("log"))
	return fss
}

func (o *Options) Complete() error {
	if err := o.Controller.Complete(); err != nil {
		return err
	}
	if err := o.Serving.Complete(); err != nil {
		return err
	}
	if err := o.Log.Complete(); err != nil {
		return err
	}

	if o.Controller.FederatedMode && o.ClusterConfigProvider == nil {
		return fmt.Errorf("cluster provider must be set when federated mode is on")
	}
	return nil
}

type subOptions interface {
	Validate() []error
	BindFlags(fs *pflag.FlagSet)
	Complete() error
}
