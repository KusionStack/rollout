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

package cert

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	certutil "github.com/zoumo/golib/cert"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
)

// GenerateSelfSignedCertKeyWithFixtures creates a self-signed certificate and key for the given host.
// Host may be an IP or a DNS name. You may also specify additional subject alt names (either ip or dns names)
// for the certificate.
//
// If fixtureDirectory is non-empty, it is a directory path which can contain pre-generated certs. The format is:
//   - tls.key: private key
//   - tls.crt: public certificate
//   - ca.crt: CA certificate
//
// Certs/keys not existing in that directory are created.
func GenerateSelfSignedCertKeyWithFixtures(host string, alternateIPs []net.IP, alternateDNS []string, fixtureDirectory string) ([]byte, []byte, []byte, error) {
	keyName := "tls.key"
	certName := "tls.crt"
	caCertName := "ca.crt"
	keyPath := filepath.Join(fixtureDirectory, keyName)
	certPath := filepath.Join(fixtureDirectory, certName)
	caCertPath := filepath.Join(fixtureDirectory, caCertName)

	if len(fixtureDirectory) > 0 {
		// try to load cert from files
		keyBytes, certBytes, caCertBytes, err := loadCertKeyAndCA(keyPath, certPath, caCertPath)
		if err == nil {
			klog.V(1).InfoS("loaded cert/key/ca", "dir", fixtureDirectory)
			return keyBytes, certBytes, caCertBytes, nil
		}
	}

	_, caCert, key, cert, err := generateSelfSignedCertKey(host, alternateIPs, alternateDNS)
	if err != nil {
		return nil, nil, nil, err
	}

	klog.V(1).InfoS("generate cert/key/ca", "host", host, "ips", alternateIPs, "dnses", alternateDNS)

	keyPEM := certutil.MarshalRSAPrivateKeyToPEM(key)
	cerPEM := certutil.MarshalCertToPEM(cert)
	caCertPEM := certutil.MarshalCertToPEM(caCert)

	if len(fixtureDirectory) > 0 {
		err := os.MkdirAll(fixtureDirectory, 0o755)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create directory %s: %v", fixtureDirectory, err)
		}
		err = keyPEM.WriteFile(keyPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to write key to %s: %v", keyPath, err)
		}
		err = cerPEM.WriteFile(certPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to write cert to %s: %v", certPath, err)
		}
		err = caCertPEM.WriteFile(caCertPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to write ca cert to %s: %v", caCertPath, err)
		}
	}
	return keyPEM.EncodeToMemory(), cerPEM.EncodeToMemory(), caCertPEM.EncodeToMemory(), nil
}

func generateSelfSignedCertKey(host string, alternateIPs []net.IP, alternateDNS []string) (*rsa.PrivateKey, *x509.Certificate, *rsa.PrivateKey, *x509.Certificate, error) {
	caKey, err := certutil.NewRSAPrivateKey()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	caCert, err := certutil.NewSelfSignedCACert(certutil.Config{
		CommonName: fmt.Sprintf("%s-ca@%d", host, time.Now().Unix()),
	}, caKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	key, err := certutil.NewRSAPrivateKey()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	cfg := certutil.Config{
		CommonName: host,
		AltNames: certutil.AltNames{
			IPs:      alternateIPs,
			DNSNames: alternateDNS,
		},
	}

	if ip := net.ParseIP(host); ip != nil {
		cfg.AltNames.IPs = append(cfg.AltNames.IPs, ip)
	} else {
		cfg.AltNames.DNSNames = append(cfg.AltNames.DNSNames, host)
	}

	cert, err := certutil.NewSignedCert(cfg, key, caKey, caCert)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return caKey, caCert, key, cert, nil
}

func loadCertKeyAndCA(keyPath, certPath, caCertPath string) ([]byte, []byte, []byte, error) {
	errList := []error{}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		errList = append(errList, err)
	}
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		errList = append(errList, err)
	}
	caBytes, err := os.ReadFile(caCertPath)
	if err != nil {
		errList = append(errList, err)
	}

	aerr := errors.NewAggregate(errList)
	if aerr != nil {
		return nil, nil, nil, aerr
	}
	return keyBytes, certBytes, caBytes, nil
}
