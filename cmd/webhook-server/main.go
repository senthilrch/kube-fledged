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

package main

import (
	"flag"

	"github.com/senthilrch/kube-fledged/cmd/webhook-server/app"
)

var (
	certFile   string
	keyFile    string
	port       int
	initServer bool
)

func init() {
	flag.StringVar(&certFile, "cert-file", "", "File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert).")
	flag.StringVar(&keyFile, "key-file", "", "File containing the default x509 private key matching --cert-file.")
	flag.IntVar(&port, "port", 443, "Secure port that the webhook server listens on")
	flag.BoolVar(&initServer, "init-server", false, "True means only init tasks for the server will be performed. Server is not started")
}

func main() {
	flag.Parse()
	if initServer {
		/*
			Call function to perform init tasks:
			- create CA cert and key
			- create server cert and key and copy to /etc/webhook/certs
			- patch validatingwebhookconfiguration with CA bundle
		*/
		if err := app.InitWebhookServer(); err != nil {
			panic(err)
		}
		return
	}
	if err := app.StartWebhookServer(certFile, keyFile, port); err != nil {
		panic(err)
	}
}
