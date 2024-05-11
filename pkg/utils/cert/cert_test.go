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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zoumo/golib/cert"
)

func TestServingCerts_Validate(t *testing.T) {
	cfg := Config{
		CommonName: "foo.example.com",
		AltNames: cert.AltNames{
			DNSNames: []string{"bar.example.com"},
		},
	}
	certs, err := GenerateSelfSignedCerts(cfg)
	assert.Nil(t, err)
	assert.Nil(t, certs.Validate("foo.example.com"))
	assert.Nil(t, certs.Validate("bar.example.com"))
	assert.NotNil(t, certs.Validate("unknown.example.com"))
}

func TestGenerateSelfSignedCerts(t *testing.T) {
	cfg := Config{
		CommonName: "rollout.rollout-system.svc",
		AltNames: cert.AltNames{
			DNSNames: []string{"rollout.rollout-system.svc", "foo.example.com"},
		},
	}
	certs, err := GenerateSelfSignedCerts(cfg)
	assert.Nil(t, err)

	err = certs.Validate("rollout.rollout-system.svc")
	assert.Nil(t, err)

	err = certs.Validate("foo.example.com")
	assert.Nil(t, err)
}
