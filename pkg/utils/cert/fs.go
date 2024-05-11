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
	"bytes"
	"context"
	"fmt"
	"os"
	"path"

	"github.com/spf13/afero"
	"k8s.io/klog/v2"
)

type FSProvider struct {
	FSOptions
	path string
}

type FSOptions struct {
	FS         afero.Fs
	CertName   string
	KeyName    string
	CAKeyName  string
	CACertName string
}

func (o *FSOptions) setDefaults() {
	if o.FS == nil {
		o.FS = afero.NewOsFs()
	}
	if len(o.CertName) == 0 {
		o.CertName = "tls.crt"
	}
	if len(o.KeyName) == 0 {
		o.KeyName = "tls.key"
	}
	if len(o.CAKeyName) == 0 {
		o.CAKeyName = "ca.key"
	}
	if len(o.CACertName) == 0 {
		o.CACertName = "ca.crt"
	}
}

func NewFSProvider(path string, opts FSOptions) (*FSProvider, error) {
	opts.setDefaults()

	if len(path) == 0 {
		return nil, fmt.Errorf("cert path is required")
	}

	return &FSProvider{
		path:      path,
		FSOptions: opts,
	}, nil
}

func (p *FSProvider) Ensure(_ context.Context, cfg Config) error {
	certs, err := p.Load()
	if err != nil && !IsNotFound(err) {
		return err
	}

	if IsNotFound(err) {
		certs, err := GenerateSelfSignedCerts(cfg)
		if err != nil {
			return err
		}
		_, err = p.Overwrite(certs)
		return err
	}

	err = certs.Validate(cfg.CommonName)
	if err != nil {
		// re-generate if expired or invalid
		klog.Info("certificates are invalid, regenerating...")
		certs, err := GenerateSelfSignedCerts(cfg)
		if err != nil {
			return err
		}
		_, err = p.Overwrite(certs)
		return err
	}
	return nil
}

func (p *FSProvider) checkIfExist() error {
	files := []string{
		path.Join(p.path, p.KeyName),
		path.Join(p.path, p.CertName),
		path.Join(p.path, p.CACertName),
		path.Join(p.path, p.CAKeyName),
	}
	for _, file := range files {
		_, err := p.FS.Stat(file)
		if err == nil {
			continue
		}

		if os.IsNotExist(err) {
			return newNotFound(file, err)
		}
		return err
	}
	return nil
}

func (p *FSProvider) Load() (*ServingCerts, error) {
	err := p.checkIfExist()
	if err != nil {
		return nil, err
	}

	keyBytes, err := afero.ReadFile(p.FS, path.Join(p.path, p.KeyName))
	if err != nil {
		return nil, err
	}
	certBytes, err := afero.ReadFile(p.FS, path.Join(p.path, p.CertName))
	if err != nil {
		return nil, err
	}
	caBytes, err := afero.ReadFile(p.FS, path.Join(p.path, p.CACertName))
	if err != nil {
		return nil, err
	}
	caKeyBytes, err := afero.ReadFile(p.FS, path.Join(p.path, p.CAKeyName))
	if err != nil {
		return nil, err
	}

	certs := &ServingCerts{
		Key:    keyBytes,
		Cert:   certBytes,
		CAKey:  caKeyBytes,
		CACert: caBytes,
	}

	return certs, nil
}

func (p *FSProvider) Overwrite(certs *ServingCerts) (bool, error) {
	if certs == nil {
		return false, fmt.Errorf("certs are required")
	}

	stat, err := p.FS.Stat(p.path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if os.IsNotExist(err) {
		err = os.MkdirAll(p.path, 0o755)
		if err != nil {
			return false, err
		}
		stat, _ = p.FS.Stat(p.path)
	}

	if !stat.IsDir() {
		return false, fmt.Errorf("the cert path %s must be a directory", p.path)
	}

	keyPath := path.Join(p.path, p.KeyName)
	var updated bool
	changed, err := p.writeFile(keyPath, certs.Key)
	if err != nil {
		return false, fmt.Errorf("failed to write key to %s: %v", keyPath, err)
	}
	updated = changed || updated

	certPath := path.Join(p.path, p.CertName)
	changed, err = p.writeFile(certPath, certs.Cert)
	if err != nil {
		return false, fmt.Errorf("failed to write cert to %s: %v", certPath, err)
	}
	updated = changed || updated

	caKeyPath := path.Join(p.path, p.CAKeyName)
	changed, err = p.writeFile(caKeyPath, certs.CAKey)
	if err != nil {
		return false, fmt.Errorf("failed to write ca key to %s: %v", caKeyPath, err)
	}
	updated = changed || updated

	caCertPath := path.Join(p.path, p.CACertName)
	changed, err = p.writeFile(caCertPath, certs.CACert)
	if err != nil {
		return false, fmt.Errorf("failed to write ca cert to %s: %v", caCertPath, err)
	}
	updated = changed || updated
	return updated, nil
}

func (p *FSProvider) writeFile(path string, data []byte) (bool, error) {
	_, err := p.FS.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if os.IsNotExist(err) {
		// created
		return true, afero.WriteFile(p.FS, path, data, 0o644)
	}
	// check data
	inFile, _ := afero.ReadFile(p.FS, path)

	if bytes.Equal(data, inFile) {
		return false, nil
	}
	// changed
	return true, afero.WriteFile(p.FS, path, data, 0o644)
}
