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
	"github.com/spf13/pflag"
)

var _ subOptions = &ServingOptions{}

type ServingOptions struct {
	MetricsBindAddress     string
	HealthProbeBindAddress string

	WebhookPort    int
	WebhookCertDir string
}

func NewServingOptions() *ServingOptions {
	return &ServingOptions{
		MetricsBindAddress:     ":8080",
		HealthProbeBindAddress: ":8081",
		WebhookPort:            9443,
	}
}

// BindFlags implements suboptions.
func (o *ServingOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.MetricsBindAddress, "metrics-bind-address", o.MetricsBindAddress, "The address the metric endpoint binds to.")
	fs.StringVar(&o.HealthProbeBindAddress, "health-probe-bind-address", o.HealthProbeBindAddress, "The address the probe endpoint binds to.")
	fs.IntVar(&o.WebhookPort, "webhook-port", o.WebhookPort, "The port that the webhook server serves at.")
	fs.StringVar(&o.WebhookCertDir, "webhook-cert-dir", o.WebhookCertDir, "The directory where the TLS certs are located. If not set, webhook server would look up the server key and certificate in {TempDir}/k8s-webhook-server/serving-certs.")
}

// Complete implements suboptions.
func (o *ServingOptions) Complete() error {
	return nil
}

// Validate implements suboptions.
func (o *ServingOptions) Validate() []error {
	return nil
}
