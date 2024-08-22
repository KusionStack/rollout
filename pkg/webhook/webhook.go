package webhook

import (
	"path"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"kusionstack.io/rollout/pkg/utils/cert"
	"kusionstack.io/rollout/pkg/webhook/controller"
)

var webhookInitializerOnce sync.Once

// setupWebhook sets up the webhook with a manager.
func setupWebhook(mgr ctrl.Manager, webhookType webhookType, gk schema.GroupKind, handler admission.Handler) {
	kind := strings.ToLower(gk.Kind)
	hookPath := path.Join("/webhooks", string(webhookType), kind)

	server := mgr.GetWebhookServer()
	server.Register(hookPath, &admission.Webhook{Handler: handler})

	webhookInitializerOnce.Do(func() {
		err := initializeWebhookCerts(mgr)
		if err != nil {
			panic("unable to initialize webhook: " + err.Error())
		}
	})
}

func initializeWebhookCerts(mgr ctrl.Manager) error {
	// NOTE: firstly generate self signed cert anyway, so that we can start the server without waiting for the cert to be ready.
	// The webhook certs will be synced by the controller later.
	server := mgr.GetWebhookServer()
	cfg := cert.Config{
		CommonName: getWebhookHost(),
		AltNames: cert.AltNames{
			DNSNames: getWebhookAlternateHosts(),
		},
	}
	err := cert.GenerateSelfSignedCertKeyIfNotExist(server.CertDir, cfg)
	if err != nil {
		return err
	}

	if isSyncWebhookCertsEnabled() {
		webhookctrl := controller.New(mgr, controller.CertConfig{
			Host:                  getWebhookHost(),
			AlternateHosts:        getWebhookAlternateHosts(),
			Namespace:             getWebhookNamespace(),
			SecretName:            getWebhookSecretName(),
			MutatingWebhookName:   mutatingWebhookConfigurationName,
			ValidatingWebhookName: validatingWebhookConfigurationName,
		})
		err := webhookctrl.SetupWithManager(mgr)
		if err != nil {
			return err
		}
	}
	return nil
}
