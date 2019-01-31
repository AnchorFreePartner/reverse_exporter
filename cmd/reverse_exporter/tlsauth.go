package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"github.com/pkg/errors"
)

func tlsAuthConfig(caCert string) (*tls.Config, error) {
	certBytes, err := ioutil.ReadFile(caCert)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to read CA cert %q", caCert)
	}

	clientCertPool := x509.NewCertPool()
	if ok := clientCertPool.AppendCertsFromPEM(certBytes); !ok {
		return nil, errors.New("Unable to add certificate to certificate pool")
	}

	tlsConfig := &tls.Config{
		ClientAuth:               tls.RequireAndVerifyClientCert,
		ClientCAs:                clientCertPool,
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
	}

	tlsConfig.BuildNameToCertificate()

	return tlsConfig, nil
}
