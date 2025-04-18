package base

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfig .
type TLSConfig struct {
	Address    string // Address
	CertFile   string // Client certificate file
	KeyFile    string // Client private key file
	CAFile     string // CA certificate file
	ServerName string // Server name (optional)
}

// CreateTLSConfig create TLS configuration
func CreateTLSConfig(tlsConfig *TLSConfig) (*tls.Config, error) {
	if tlsConfig == nil {
		return nil, nil
	}

	// Load the client certificate and private key
	cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("the client certificate cannot be loaded: %v", err)
	}

	// Load the CA certificate
	caCert, err := os.ReadFile(tlsConfig.CAFile)
	if err != nil {
		return nil, fmt.Errorf("the CA certificate cannot be read: %v", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("the CA certificate cannot be parsed")
	}

	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: false,
	}

	if tlsConfig.ServerName != "" {
		tlsConf.ServerName = tlsConfig.ServerName
	}

	return tlsConf, nil
}
