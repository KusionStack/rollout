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

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rolloututils "kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/utils/cert"
)

type WebhookCertSyncer struct {
	*mixin.ReconcilerMixin
	CertConfig

	fs     *cert.FSProvider
	secret *cert.SecretProvider
}

type CertConfig struct {
	Host                  string
	AlternateHosts        []string
	Namespace             string
	SecretName            string
	MutatingWebhookName   string
	ValidatingWebhookName string
}

func New(mgr manager.Manager, cfg CertConfig) *WebhookCertSyncer {
	return &WebhookCertSyncer{
		ReconcilerMixin: mixin.NewReconcilerMixin("webhook-cert-syncer", mgr),
		CertConfig:      cfg,
	}
}

func (c *WebhookCertSyncer) SetupWithManager(mgr manager.Manager) error {
	var err error
	server := mgr.GetWebhookServer()
	c.fs, err = cert.NewFSProvider(server.CertDir, cert.FSOptions{
		CertName: server.CertName,
		KeyName:  server.KeyName,
	})
	if err != nil {
		return err
	}
	c.secret, err = cert.NewSecretProvider(
		&secretClient{
			reader: c.APIReader,
			writer: c.Client,
			logger: c.Logger,
		},
		c.Namespace,
		c.SecretName,
	)
	if err != nil {
		return err
	}

	// manually sync certs once
	_, err = c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: c.Namespace,
			Name:      c.SecretName,
		},
	})
	if err != nil {
		return err
	}
	ctrl, _ := controller.NewUnmanaged(c.GetControllerName(), mgr, controller.Options{
		Reconciler: c,
	})

	// add watches for secrets, webhook configs
	types := []client.Object{
		&corev1.Secret{},
		&admissionregistrationv1.ValidatingWebhookConfiguration{},
		&admissionregistrationv1.MutatingWebhookConfiguration{},
	}
	for i := range types {
		t := types[i]
		err = ctrl.Watch(
			multicluster.FedKind(&source.Kind{Type: t}),
			c.enqueueSecret(),
			predicate.NewPredicateFuncs(c.isInterestedObject),
		)
		if err != nil {
			return err
		}
	}
	// make controller run as non-leader election
	return mgr.Add(&nonLeaderElectionController{Controller: ctrl})
}

func (c *WebhookCertSyncer) isInterestedObject(obj client.Object) bool {
	if obj == nil {
		return false
	}
	switch t := obj.(type) {
	case *corev1.Secret:
		return t.Namespace == c.Namespace && t.Name == c.SecretName
	case *admissionregistrationv1.ValidatingWebhookConfiguration:
		return t.Name == c.ValidatingWebhookName
	case *admissionregistrationv1.MutatingWebhookConfiguration:
		return t.Name == c.MutatingWebhookName
	}
	return false
}

func (c *WebhookCertSyncer) enqueueSecret() handler.EventHandler {
	mapFunc := func(obj client.Object) []reconcile.Request {
		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: c.Namespace,
					Name:      c.SecretName,
				},
			},
		}
	}
	return handler.EnqueueRequestsFromMapFunc(mapFunc)
}

func (c *WebhookCertSyncer) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	servingCerts, err := c.secret.Ensure(ctx, c.Host, c.AlternateHosts)
	if err != nil {
		if cert.IsConflict(err) {
			// create error on AlreadyExists
			// update error on Conflict
			// retry
			return reconcile.Result{RequeueAfter: 1 * time.Second}, nil
		}
		return reconcile.Result{}, err
	}

	if servingCerts == nil {
		return reconcile.Result{}, fmt.Errorf("got empty serving certs from secret")
	}

	// got valid serving certs in secret
	// 1. write certs to fs
	changed, err := c.fs.Overwrite(servingCerts)
	if err != nil {
		return reconcile.Result{}, err
	}
	if changed {
		c.Logger.Info("write certs to files successfully")
	}

	// 2. update caBundle in webhook configurations
	err = c.ensureWebhookConfiguration(ctx, servingCerts.CACert)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (c *WebhookCertSyncer) ensureWebhookConfiguration(ctx context.Context, caBundle []byte) error {
	reader := c.APIReader
	writer := c.Client

	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	mutatingCfg := &admissionregistrationv1.MutatingWebhookConfiguration{}
	err := reader.Get(ctx, client.ObjectKey{Name: c.MutatingWebhookName}, mutatingCfg)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if err == nil {
		changed, err := rolloututils.UpdateOnConflict(ctx, reader, writer, mutatingCfg, func() error {
			for i := range mutatingCfg.Webhooks {
				mutatingCfg.Webhooks[i].ClientConfig.CABundle = caBundle
			}
			return nil
		})
		if err != nil {
			c.Logger.Info("failed to update ca in mutating webhook", "error", err.Error())
		} else if changed {
			c.Logger.Info("ensure ca in mutating webhook", "changed", changed)
		}
	}
	validatingCfg := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	err = reader.Get(ctx, client.ObjectKey{Name: c.ValidatingWebhookName}, validatingCfg)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if err == nil {
		changed, err := rolloututils.UpdateOnConflict(ctx, reader, writer, validatingCfg, func() error {
			for i := range validatingCfg.Webhooks {
				validatingCfg.Webhooks[i].ClientConfig.CABundle = caBundle
			}
			return nil
		})

		if err != nil {
			c.Logger.Info("failed to update ca in validating webhook", "error", err.Error())
		} else if changed {
			c.Logger.Info("ensure ca in validating webhook", "changed", changed)
		}
	}
	return nil
}

type nonLeaderElectionController struct {
	controller.Controller
}

func (c *nonLeaderElectionController) NeedLeaderElection() bool {
	return false
}

var _ cert.SecretClient = &secretClient{}

type secretClient struct {
	reader client.Reader
	writer client.Writer
	logger logr.Logger
}

// Create implements cert.SecretClient.
func (s *secretClient) Create(ctx context.Context, secret *corev1.Secret) error {
	err := s.writer.Create(ctx, secret)
	if err == nil {
		s.logger.Info("create secret successfully", "namespace", secret.Namespace, "name", secret.Name)
	}
	return err
}

// Get implements cert.SecretClient.
func (s *secretClient) Get(ctx context.Context, namespace string, name string) (*corev1.Secret, error) {
	var secret corev1.Secret
	err := s.reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &secret)
	if err != nil {
		return nil, err
	}
	return &secret, nil
}

// Update implements cert.SecretClient.
func (s *secretClient) Update(ctx context.Context, secret *corev1.Secret) error {
	err := s.writer.Update(ctx, secret)
	if err == nil {
		s.logger.Info("update secret successfully", "namespace", secret.Namespace, "name", secret.Name)
	}
	return err
}
