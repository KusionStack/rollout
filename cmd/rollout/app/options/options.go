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
	"flag"
	"fmt"
	"strings"

	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2/klogr"
	"kusionstack.io/kube-utils/controller/initializer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"kusionstack.io/rollout/pkg/features"
)

type Options struct {
	MetricsBindAddress     string
	HealthProbeBindAddress string
	LeaderElect            bool
	FederatedMode          bool
	Logger                 string
	ZapOptions             *zap.Options
}

func NewOptions() *Options {
	return &Options{
		MetricsBindAddress:     ":8080",
		HealthProbeBindAddress: ":8081",
		LeaderElect:            false,
		FederatedMode:          true,
		Logger:                 "zap",
		ZapOptions: &zap.Options{
			Development: true,
		},
	}
}

func (o *Options) Validate() []error {
	var errs []error

	switch o.Logger {
	case "zap", "klog":
	default:
		errs = append(errs, fmt.Errorf("invalid logger type %q", o.Logger))
	}

	return errs
}

func (o *Options) Flags(initializer initializer.Interface) cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}
	fs := fss.FlagSet("options")

	fs.StringVar(&o.MetricsBindAddress, "metrics-bind-address", o.MetricsBindAddress, "The address the metric endpoint binds to.")
	fs.StringVar(&o.HealthProbeBindAddress, "health-probe-bind-address", o.HealthProbeBindAddress, "The address the probe endpoint binds to.")
	fs.BoolVar(&o.LeaderElect, "leader-elect", o.LeaderElect,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.BoolVar(&o.FederatedMode, "federated-mode", o.FederatedMode, "Enable federated mode for controller manager.")
	fs.StringVar(&o.Logger, "logger", o.Logger, "The logger provider, Options are:\n"+strings.Join([]string{"zap", "klog"}, "\n"))

	// bind zap flags
	zapFs := flag.NewFlagSet("zap", flag.ExitOnError)
	o.ZapOptions.BindFlags(zapFs)
	zapPfs := fss.FlagSet("zap")
	zapPfs.AddGoFlagSet(zapFs)

	// add feature gate flags
	features.DefaultMutableFeatureGate.AddFlag(fs)

	// bind initializer flags
	initializer.BindFlag(fs)
	return fss
}

func (o *Options) Complete() error {
	switch o.Logger {
	case "zap":
		ctrl.SetLogger(zap.New(zap.UseFlagOptions(o.ZapOptions)))
	default:
		ctrl.SetLogger(klogr.New())
	}
	return nil
}