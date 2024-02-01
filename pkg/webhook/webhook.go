package webhook

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	admv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/kubernetes/test/utils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloututils "kusionstack.io/rollout/pkg/utils"
)

const (
	mutatingWebhookConfigurationName   = "kusionstack-rollout-mutating"
	validatingWebhookConfigurationName = "kusionstack-rollout-validating"
	webhookCertsSecretName             = "rollout-webhook-certs"
)

var webhookInitializerOnce sync.Once

type RegisterHandlerFunc func(manager ctrl.Manager)

// SetupWithManager sets up the webhook with a manager.
func SetupWithManager(mgr ctrl.Manager, f RegisterHandlerFunc) (bool, error) {
	f(mgr)

	webhookInitializerOnce.Do(func() {
		err := initialize(context.Background(), mgr)
		if err != nil {
			panic("unable to initialize webhook: " + err.Error())
		}
	})

	return true, nil
}

func initialize(ctx context.Context, mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("webhook")

	secret, err := ensureWebhookSecret(ctx, mgr.GetAPIReader(), mgr.GetClient(), getWebhookHost())
	if err != nil {
		return err
	}
	logger.Info("webhook secret ensured", "namespace", secret.Namespace, "name", secret.Name)

	mutatingCfg := &admv1.MutatingWebhookConfiguration{}
	err = mgr.GetAPIReader().Get(ctx, client.ObjectKey{Name: mutatingWebhookConfigurationName}, mutatingCfg)
	if err != nil {
		return err
	}

	_, err = rolloututils.UpdateOnConflict(ctx, mgr.GetAPIReader(), mgr.GetClient(), mutatingCfg, func() error {
		for i := range mutatingCfg.Webhooks {
			mutatingCfg.Webhooks[i].ClientConfig.CABundle = secret.Data["ca.crt"]
		}
		return nil
	})
	if err != nil {
		return err
	}

	validatingCfg := &admv1.ValidatingWebhookConfiguration{}
	err = mgr.GetAPIReader().Get(ctx, client.ObjectKey{Name: validatingWebhookConfigurationName}, validatingCfg)
	if err != nil {
		return err
	}

	_, err = rolloututils.UpdateOnConflict(ctx, mgr.GetAPIReader(), mgr.GetClient(), validatingCfg, func() error {
		for i := range validatingCfg.Webhooks {
			if validatingCfg.Webhooks[i].ClientConfig.CABundle == nil {
				validatingCfg.Webhooks[i].ClientConfig.CABundle = secret.Data["ca.crt"]
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	logger.Info("webhook ca bundle ensured", "mutatingwebhookconfiguration", mutatingWebhookConfigurationName, "validatingwebhookconfiguration", validatingWebhookConfigurationName)

	var tlsKey, tlsCert []byte
	tlsKey, ok := secret.Data["tls.key"]
	if !ok {
		return errors.New("tls.key not found in secret")
	}
	tlsCert, ok = secret.Data["tls.crt"]
	if !ok {
		return errors.New("tls.crt not found in secret")
	}

	err = ensureWebhookCert(mgr.GetWebhookServer().CertDir, tlsKey, tlsCert)
	if err != nil {
		return err
	}
	logger.Info("webhook ensured")
	return nil
}

func ensureWebhookSecret(ctx context.Context, reader client.Reader, writer client.Writer, commonName string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := reader.Get(ctx, client.ObjectKey{Namespace: getNamespace(), Name: webhookCertsSecretName}, secret)
	if err == nil {
		return secret, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	// create secret if not found
	caKey, caCert, err := generateSelfSignedCACert()
	if err != nil {
		return nil, err
	}
	caKeyPEM, err := keyutil.MarshalPrivateKeyToPEM(caKey)
	if err != nil {
		return nil, err
	}
	caCertPEM := utils.EncodeCertPEM(caCert)

	privateKey, signedCert, err := generateSelfSignedCert(caCert, caKey, commonName)
	if err != nil {
		return nil, err
	}
	privateKeyPEM, err := keyutil.MarshalPrivateKeyToPEM(privateKey)
	if err != nil {
		return nil, err
	}
	signedCertPEM := utils.EncodeCertPEM(signedCert)

	data := map[string][]byte{
		"ca.key": caKeyPEM, "ca.crt": caCertPEM,
		"tls.key": privateKeyPEM, "tls.crt": signedCertPEM,
	}
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookCertsSecretName,
			Namespace: getNamespace(),
		},
		Data: data,
	}

	err = writer.Create(ctx, secret)
	return secret, err
}

func generateSelfSignedCACert() (caKey *rsa.PrivateKey, caCert *x509.Certificate, err error) {
	caKey, err = utils.NewPrivateKey()
	if err != nil {
		return
	}

	caCert, err = cert.NewSelfSignedCACert(cert.Config{CommonName: "self-signed-k8s-cert"}, caKey)

	return
}

func generateSelfSignedCert(caCert *x509.Certificate, caKey crypto.Signer, commonName string) (privateKey *rsa.PrivateKey, signedCert *x509.Certificate, err error) {
	privateKey, err = utils.NewPrivateKey()
	if err != nil {
		return
	}

	hostIP := net.ParseIP(commonName)
	var altIPs []net.IP
	DNSNames := []string{"localhost"}
	if hostIP.To4() != nil {
		altIPs = append(altIPs, hostIP.To4())
	} else {
		DNSNames = append(DNSNames, commonName)
	}

	signedCert, err = utils.NewSignedCert(
		&cert.Config{
			CommonName: commonName,
			AltNames:   cert.AltNames{DNSNames: DNSNames, IPs: altIPs},
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		},
		privateKey, caCert, caKey,
	)

	return
}

func ensureWebhookCert(certDir string, tlsKey, tlsCert []byte) error {
	if _, err := os.Stat(certDir); os.IsNotExist(err) {
		err := os.MkdirAll(certDir, 0777)
		if err != nil {
			return err
		}
	}

	keyFile := filepath.Join(certDir, "tls.key")
	certFile := filepath.Join(certDir, "tls.crt")

	if err := os.WriteFile(keyFile, tlsKey, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(certFile, tlsCert, 0644); err != nil {
		return err
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
