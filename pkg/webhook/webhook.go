package webhook

import (
	"net/http"
	"path"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"kusionstack.io/rollout/pkg/utils/cert"
	"kusionstack.io/rollout/pkg/webhook/controller"
)

var webhookInitializerOnce sync.Once

type RegisterHandlerFunc func(manager *webhook.Server)

// setupWebhook sets up the webhook with a manager.
func setupWebhook(mgr ctrl.Manager, webhookType webhookType, obj runtime.Object, handler http.Handler) error {
	// NOTE: we must register the webhook before initializing cert and configuration.
	scheme := mgr.GetScheme()
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}
	gvk := gvks[0]
	kind := strings.ToLower(gvk.Kind)
	hookPath := path.Join("/webhooks", string(webhookType), kind)

	server := mgr.GetWebhookServer()
	server.Register(hookPath, handler)

	webhookInitializerOnce.Do(func() {
		err := initializeWebhookCerts(mgr)
		if err != nil {
			panic("unable to initialize webhook: " + err.Error())
		}
	})

	return nil
}

func initializeWebhookCerts(mgr ctrl.Manager) error {
	// NOTE: firstly generate self signed cert anyway, so that we can start the server without waiting for the cert to be ready.
	// The webhook certs will be synced by the controller later.
	server := mgr.GetWebhookServer()
	err := cert.GenerateSelfSignedCertKeyIfNotExist(server.CertDir, getWebhookHost(), getWebhookAlternateHosts())
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
