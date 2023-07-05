/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
limitations under the License.
*/
package barbican

import (
	"context"
	"encoding/json"
	"os"

	// nolint
	brb "github.com/artashesbalabekyan/barbican-sdk-go"
	"github.com/artashesbalabekyan/barbican-sdk-go/client"
	"github.com/artashesbalabekyan/barbican-sdk-go/xhttp"
	. "github.com/onsi/ginkgo/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	// nolint

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/external-secrets/external-secrets-e2e/framework"
	esv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	esmeta "github.com/external-secrets/external-secrets/apis/meta/v1"
)

// nolint // Better to keep names consistent even if it stutters;
type Provider struct {
	framework *framework.Framework
	config    *xhttp.Config
	client    client.Conn
}

func NewProvider(f *framework.Framework, config *xhttp.Config) *Provider {
	client, err := brb.NewFakeConnection(context.Background(), nil)
	if err != nil {
		Fail(err.Error())
	}
	prov := &Provider{
		client:    client,
		config:    config,
		framework: f,
	}

	BeforeEach(func() {
		prov.CreateSAKeyStore()
		prov.CreateSpecifcSASecretStore()
		prov.CreatePodIDStore()
	})

	AfterEach(func() {
		prov.DeleteSpecifcSASecretStore()
	})

	return prov
}

func NewFromEnv(f *framework.Framework) *Provider {
	var config xhttp.Config
	config.Endpoint = os.Getenv("BARBICAN_ENDPOINT")
	config.Login.ProjectDomain = os.Getenv("BARBICAN_PROJECT_DOMAIN")
	config.Login.ProjectName = os.Getenv("BARBICAN_PROJECT_NAME")
	config.Login.AuthUrl = os.Getenv("BARBICAN_AUTH_URL")
	config.Login.Username = os.Getenv("BARBICAN_USERNAME")
	config.Login.Password = os.Getenv("BARBICAN_PASSWORD")
	config.Login.UserDomainName = os.Getenv("BARBICAN_USER_DOMAIN_NAME")
	return NewProvider(f, &config)
}

func (s *Provider) getClient(ctx context.Context) (client client.Conn, err error) {
	client, err = brb.NewFakeConnection(context.Background(), nil)
	if err != nil {
		Fail(err.Error())
	}
	return client, err
}

func (s *Provider) CreateSecret(key string, val framework.SecretEntry) {
	ctx := context.Background()
	client, err := s.getClient(ctx)
	Expect(err).ToNot(HaveOccurred())

	err = client.Create(ctx, key, []byte(val.Value))
	Expect(err).ToNot(HaveOccurred())

}

func (s *Provider) DeleteSecret(key string) {
	ctx := context.Background()
	client, err := s.getClient(ctx)
	Expect(err).ToNot(HaveOccurred())
	err = client.DeleteSecret(ctx, key)
	Expect(err).ToNot(HaveOccurred())
}

func makeStore(s *Provider) *esv1beta1.SecretStore {
	return &esv1beta1.SecretStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.framework.Namespace.Name,
			Namespace: s.framework.Namespace.Name,
		},
		Spec: esv1beta1.SecretStoreSpec{
			Provider: &esv1beta1.SecretStoreProvider{
				Barbican: &esv1beta1.BarbicanProvider{
					Auth: esv1beta1.BarbicanAuth{
						SecretRef: &esv1beta1.BarbicanAuthSecretRef{
							SecretAccessKey: esmeta.SecretKeySelector{
								Name: staticCredentialsSecretName,
								Key:  serviceAccountKey,
							},
						},
					},
				},
			},
		},
	}
}

const (
	serviceAccountKey           = "secret-access-credentials"
	PodIDSecretStoreName        = "pod-identity"
	staticCredentialsSecretName = "provider-secret"
)

func makeCredsJSONFromConfig(config *xhttp.Config) string {
	js, err := json.Marshal(config)
	if err != nil {
		Fail(err.Error())
	}
	return string(js)
}

func (s *Provider) CreateSAKeyStore() {
	brbCreds := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      staticCredentialsSecretName,
			Namespace: s.framework.Namespace.Name,
		},
		StringData: map[string]string{
			serviceAccountKey: makeCredsJSONFromConfig(s.config),
		},
	}
	err := s.framework.CRClient.Create(context.Background(), brbCreds)
	if err != nil {
		err = s.framework.CRClient.Update(context.Background(), brbCreds)
		Expect(err).ToNot(HaveOccurred())
	}
	secretStore := makeStore(s)
	secretStore.Spec.Provider.Barbican.Auth = esv1beta1.BarbicanAuth{
		SecretRef: &esv1beta1.BarbicanAuthSecretRef{
			SecretAccessKey: esmeta.SecretKeySelector{
				Name: staticCredentialsSecretName,
				Key:  serviceAccountKey,
			},
		},
	}
	err = s.framework.CRClient.Create(context.Background(), secretStore)
	Expect(err).ToNot(HaveOccurred())
}

func (s *Provider) CreatePodIDStore() {
	secretStore := makeStore(s)
	secretStore.ObjectMeta.Name = PodIDSecretStoreName
	err := s.framework.CRClient.Create(context.Background(), secretStore)
	Expect(err).ToNot(HaveOccurred())
}

func (s *Provider) SAClusterSecretStoreName() string {
	return s.framework.Namespace.Name
}

func (s *Provider) CreateSpecifcSASecretStore() {
	clusterSecretStore := &esv1beta1.ClusterSecretStore{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.SAClusterSecretStoreName(),
		},
	}
	_, err := controllerutil.CreateOrUpdate(context.Background(), s.framework.CRClient, clusterSecretStore, func() error {
		clusterSecretStore.Spec.Provider = &esv1beta1.SecretStoreProvider{
			Barbican: &esv1beta1.BarbicanProvider{
				Auth: esv1beta1.BarbicanAuth{
					SecretRef: &esv1beta1.BarbicanAuthSecretRef{
						SecretAccessKey: esmeta.SecretKeySelector{
							Name: staticCredentialsSecretName,
							Key:  serviceAccountKey,
						},
					},
				},
			},
		}
		return nil
	})
	Expect(err).ToNot(HaveOccurred())
}

// Cleanup removes global resources that may have been
// created by this provider.
func (s *Provider) DeleteSpecifcSASecretStore() {
	err := s.framework.CRClient.Delete(context.Background(), &esv1beta1.ClusterSecretStore{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.SAClusterSecretStoreName(),
		},
	})
	Expect(err).ToNot(HaveOccurred())
}
