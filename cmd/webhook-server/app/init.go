/*
Copyright 2018 The kube-fledged authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"time"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func InitWebhookServer() error {
	var caPEM, serverCertPEM, serverPrivKeyPEM *bytes.Buffer

	webhookServerService := os.Getenv("WEBHOOK_SERVER_SERVICE")
	webhookServerNameSpace := os.Getenv("KUBEFLEDGED_NAMESPACE")
	certKeyPath := os.Getenv("CERT_KEY_PATH")
	validatingWebhookConfig := os.Getenv("VALIDATING_WEBHOOK_CONFIG")

	// CA config
	caConf := &x509.Certificate{
		SerialNumber: big.NewInt(2021),
		Subject: pkix.Name{
			Organization: []string{"kubefledged.io"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// CA private key
	caPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		glog.Errorf("error in generating CA private key: %v", err)
		return err
	}

	// Self signed CA certificate
	caBytes, err := x509.CreateCertificate(cryptorand.Reader, caConf, caConf, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		glog.Errorf("error in generating CA certificate: %v", err)
		return err
	}

	// PEM encode CA cert
	caPEM = new(bytes.Buffer)
	_ = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	dnsNames := []string{
		webhookServerService,
		webhookServerService + "." + webhookServerNameSpace,
		webhookServerService + "." + webhookServerNameSpace + ".svc",
		webhookServerService + "." + webhookServerNameSpace + ".svc.cluster"}
	commonName := webhookServerService + "." + webhookServerNameSpace + ".svc"

	// server cert config
	certConf := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"kubefledged.io"},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// server private key
	serverPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		glog.Errorf("error in generating server private key: %v", err)
		return err
	}

	// sign the server cert
	serverCertBytes, err := x509.CreateCertificate(cryptorand.Reader, certConf, caConf, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		glog.Errorf("error in generating server certificate: %v", err)
		return err
	}

	// PEM encode the  server cert and key
	serverCertPEM = new(bytes.Buffer)
	_ = pem.Encode(serverCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	})

	serverPrivKeyPEM = new(bytes.Buffer)
	_ = pem.Encode(serverPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	})

	err = os.MkdirAll(certKeyPath, 0666)
	if err != nil {
		glog.Errorf("error in creating directory %s: %v", certKeyPath, err)
		return err
	}
	err = writeFile(certKeyPath+"tls.crt", serverCertPEM)
	if err != nil {
		glog.Errorf("error in writing tls.crt: %v", err)
		return err
	}

	err = writeFile(certKeyPath+"tls.key", serverPrivKeyPEM)
	if err != nil {
		glog.Errorf("error in writing tls.key: %v", err)
		return err
	}

	err = patchValidatingWebhookConfig(caPEM, validatingWebhookConfig)
	if err != nil {
		return err
	}
	return nil
}

// writeFile writes data in the file at the given path
func writeFile(filepath string, sCert *bytes.Buffer) error {
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(sCert.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func patchValidatingWebhookConfig(caPEM *bytes.Buffer, validatingWebhookConfig string) error {

	cfg, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
		return err
	}

	_, err = kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(
		context.TODO(), validatingWebhookConfig, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Error in getting validatingwebhookconfig: %s", err.Error())
		return err
	}

	//  patchStringValue specifies a patch operation for a string.
	type patchStringValue struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}
	payload := []patchStringValue{{
		Op:    "replace",
		Path:  "/webhooks/0/clientConfig/caBundle",
		Value: base64.StdEncoding.EncodeToString(caPEM.Bytes()),
	}}
	payloadBytes, _ := json.Marshal(payload)

	_, err = kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Patch(
		context.TODO(), validatingWebhookConfig, types.JSONPatchType, payloadBytes,
		metav1.PatchOptions{})
	if err != nil {
		glog.Errorf("Error in patching validatingwebhookconfig: %s", err.Error())
		return err
	}

	return nil
}
