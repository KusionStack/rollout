/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package options

import (
	"flag"
	"fmt"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"kusionstack.io/rollout/pkg/utils/cli"
)

var _ subOptions = &LogOptions{}

type LogOptions struct {
	Logger     string
	ZapOptions *zap.Options
}

func NewLogOptions() *LogOptions {
	return &LogOptions{
		Logger: "klog",
		ZapOptions: &zap.Options{
			Development: true,
		},
	}
}

func (o *LogOptions) Validate() []error {
	var errs []error

	switch o.Logger {
	case "zap", "klog":
	default:
		errs = append(errs, fmt.Errorf("invalid logger type %q", o.Logger))
	}

	return errs
}

func (o *LogOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Logger, "logger", o.Logger, "The logger provider, Options are:\nzap\nklog")

	// bind zap flags
	zapFs := flag.NewFlagSet("zap", flag.ExitOnError)
	o.ZapOptions.BindFlags(zapFs)
	fs.AddGoFlagSet(zapFs)

	// bind klog flags
	cli.AddKlogFlags(fs)
}

func (o *LogOptions) Complete() error {
	switch o.Logger {
	case "zap":
		ctrl.SetLogger(zap.New(zap.UseFlagOptions(o.ZapOptions)))
	default:
		ctrl.SetLogger(klogr.New())
	}

	return nil
}
