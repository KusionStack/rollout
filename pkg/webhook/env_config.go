/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package webhook

import (
	"fmt"
	"os"
	"strings"
)

const (
	mutatingWebhookConfigurationName   = "kusionstack-rollout-mutating"
	validatingWebhookConfigurationName = "kusionstack-rollout-validating"
)

func getWebhookNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); len(ns) > 0 {
		return ns
	}
	return "rollout-system"
}

func getWebhookSecretName() string {
	if name := os.Getenv("WEBHOOK_SECRET_NAME"); len(name) > 0 {
		return name
	}
	return "kusionstack-rollout-webhook-certs"
}

func getWebhookServiceName() string {
	if name := os.Getenv("WEBHOOK_SERVICE_NAME"); len(name) > 0 {
		return name
	}
	return "kusionstack-rollout-webhook-service"
}

func getWebhookHost() string {
	if host := os.Getenv("WEBHOOK_HOST"); len(host) > 0 {
		return host
	}

	// defaults to kusionstack-rollout-webhook-service.rollout-system.svc
	return fmt.Sprintf("%s.%s.svc", getWebhookServiceName(), getWebhookNamespace())
}

func getWebhookAlternateHosts() []string {
	hosts := os.Getenv("WEBHOOK_ALTERNATE_HOSTS")
	tokens := strings.Split(hosts, ",")
	result := []string{}
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if len(t) > 0 {
			result = append(result, t)
		}
	}
	return result
}

func isSyncWebhookCertsEnabled() bool {
	enabled := os.Getenv("ENABLE_SYNC_WEBHOOK_CERTS")

	return enabled == "true" || enabled == "TRUE"
}
