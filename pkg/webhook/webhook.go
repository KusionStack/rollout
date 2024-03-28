package webhook

import (
	"context"
	"os"
	"sync"

	admv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	rolloututils "kusionstack.io/rollout/pkg/utils"
	certutil "kusionstack.io/rollout/pkg/utils/cert"
)

const (
	mutatingWebhookConfigurationName   = "kusionstack-rollout-mutating"
	validatingWebhookConfigurationName = "kusionstack-rollout-validating"
	webhookCertsSecretName             = "rollout-webhook-certs"
)

var webhookInitializerOnce sync.Once

type RegisterHandlerFunc func(manager *webhook.Server)

// SetupWithManager sets up the webhook with a manager.
func SetupWithManager(mgr ctrl.Manager, f RegisterHandlerFunc) (bool, error) {
	// NOTE: we must register the webhook before initializing cert and configuration.
	f(mgr.GetWebhookServer())

	webhookInitializerOnce.Do(func() {
		err := initializeCertKeyAndConfiguration(context.Background(), mgr)
		if err != nil {
			panic("unable to initialize webhook: " + err.Error())
		}
	})

	return true, nil
}

func initializeCertKeyAndConfiguration(ctx context.Context, mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("webhook")

	dir := mgr.GetWebhookServer().CertDir
	logger.Info("load or generate webhook serving key and cert", "certDir", dir)
	// 1. read key, cert, ca.cert from files or generate new ones if not exist
	keyBytes, certBytes, caCertBytes, err := certutil.GenerateSelfSignedCertKeyWithFixtures(getWebhookHost(), nil, nil, dir)
	if err != nil {
		return err
	}

	// 2. create secret
	secret, result, err := ensureWebhookSecret(ctx, mgr.GetAPIReader(), mgr.GetClient(), keyBytes, certBytes, caCertBytes)
	if err != nil {
		return err
	}
	logger.Info("ensure webhook secret", "namespace", secret.Namespace, "name", secret.Name, "opResult", result)

	// 3. update caBundle in webhook configurations
	err = ensureWebhookConfiguration(ctx, mgr.GetAPIReader(), mgr.GetClient(), caCertBytes)
	if err != nil {
		return err
	}

	logger.Info("ensure webhook configurations")
	return nil
}

func ensureWebhookSecret(ctx context.Context, reader client.Reader, writer client.Writer, keyBytes, certBytes, caCertBytes []byte) (*corev1.Secret, controllerutil.OperationResult, error) {
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: getNamespace(),
			Name:      webhookCertsSecretName,
		},
	}
	result, err := rolloututils.CreateOrUpdateOnConflict(ctx, reader, writer, secret, func() error {
		secret.Data = map[string][]byte{
			"tls.key": keyBytes,
			"tls.crt": certBytes,
			"ca.crt":  caCertBytes,
		}
		return nil
	})
	return secret, result, err
}

func ensureWebhookConfiguration(ctx context.Context, reader client.Reader, writer client.Writer, caBundle []byte) error {
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	mutatingCfg := &admv1.MutatingWebhookConfiguration{}
	err := reader.Get(ctx, client.ObjectKey{Name: mutatingWebhookConfigurationName}, mutatingCfg)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if err == nil {
		// nolint
		rolloututils.UpdateOnConflict(ctx, reader, writer, mutatingCfg, func() error {
			for i := range mutatingCfg.Webhooks {
				mutatingCfg.Webhooks[i].ClientConfig.CABundle = caBundle
			}
			return nil
		})
	}
	validatingCfg := &admv1.ValidatingWebhookConfiguration{}
	err = reader.Get(ctx, client.ObjectKey{Name: validatingWebhookConfigurationName}, validatingCfg)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if err == nil {
		// nolint
		rolloututils.UpdateOnConflict(ctx, reader, writer, validatingCfg, func() error {
			for i := range validatingCfg.Webhooks {
				validatingCfg.Webhooks[i].ClientConfig.CABundle = caBundle
			}
			return nil
		})
	}
	return nil
}

func getNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); len(ns) > 0 {
		return ns
	}
	return "rollout-system"
}

func getWebhookHost() string {
	if host := os.Getenv("WEBHOOK_HOST"); len(host) > 0 {
		return host
	}
	return "rollout-webhook-service.rollout-system.svc"
}
