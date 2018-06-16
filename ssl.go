package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"time"

	"github.com/cloudflare/cfssl/config"

	"novit.nc/direktil/pkg/clustersconfig"
)

const (
	// From Kubernetes:
	// ECPrivateKeyBlockType is a possible value for pem.Block.Type.
	ECPrivateKeyBlockType = "EC PRIVATE KEY"
)

func sslConfig(cfg *clustersconfig.Config) (*config.Config, error) {
	return config.LoadConfig([]byte(cfg.SSLConfig))
}

func PrivateKeyPEM() (*ecdsa.PrivateKey, []byte) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal("Failed to generate the key: ", err)
	}

	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		log.Fatal("Unable to mashal EC key: ", err)
	}

	buf := &bytes.Buffer{}

	if err := pem.Encode(buf, &pem.Block{
		Type:  ECPrivateKeyBlockType,
		Bytes: b,
	}); err != nil {
		log.Fatal("Failed to write encode key: ", err)
	}

	return key, buf.Bytes()
}

func SelfSignedCertificatePEM(ttlYears int, key *ecdsa.PrivateKey) []byte {
	notBefore := time.Now()
	notAfter := notBefore.AddDate(ttlYears, 0, 0).Truncate(24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(0xffffffff))
	if err != nil {
		log.Fatal("Failed to generate serial number: ", err)
	}

	certTemplate := &x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		Subject:               pkix.Name{},
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{},
		BasicConstraintsValid: true,
	}
	parentTemplate := certTemplate // self-signed
	publicKey := key.Public()

	derBytes, err := x509.CreateCertificate(rand.Reader, certTemplate, parentTemplate, publicKey, key)
	if err != nil {
		log.Fatal("Failed to generate certificate: ", err)
	}

	buf := &bytes.Buffer{}

	if err := pem.Encode(buf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		log.Fatal("Failed to write certificate: ", err)
	}

	return buf.Bytes()
}

func HostCertificatePEM(ttlYears int, key *ecdsa.PrivateKey, ctx *renderContext) ([]byte, error) {
	caKey, err := loadPrivateKey("ca", ctx)
	if err != nil {
		return nil, err
	}
	caCrt, err := loadCertificate("ca", ctx)
	if err != nil {
		return nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.AddDate(ttlYears, 0, 0).Truncate(24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(0xffffffff))
	if err != nil {
		log.Fatal("Failed to generate serial number: ", err)
	}

	dnsNames := []string{ctx.Host.Name}
	ips := []net.IP{net.ParseIP(ctx.Host.IP)}

	if ctx.Group.Master {
		dnsNames = append(dnsNames,
			"kubernetes",
			"kubernetes.kube-system",
			"kubernetes.kube-system.svc."+ctx.Cluster.Domain,
		)
		ips = append(ips, ctx.Cluster.KubernetesSvcIP())
	}

	certTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		IsCA:         false,
		Subject: pkix.Name{
			CommonName: ctx.Host.Name,
		},
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:    dnsNames,
		IPAddresses: ips,
	}
	parentTemplate := caCrt
	publicKey := key.Public()

	derBytes, err := x509.CreateCertificate(rand.Reader, certTemplate, parentTemplate, publicKey, caKey)
	if err != nil {
		log.Fatal("Failed to generate certificate: ", err)
	}

	f := &bytes.Buffer{}
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		log.Fatal("Failed to write certificate: ", err)
	}

	return f.Bytes(), nil
}

func loadPrivateKey(path string, ctx *renderContext) (*ecdsa.PrivateKey, error) {
	keyS, err := getSecret("tls-key:"+path, ctx)
	if err != nil {
		return nil, err
	}

	keyBytes := []byte(keyS)
	if len(keyBytes) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}

	p, _ := pem.Decode(keyBytes)
	if p.Type != ECPrivateKeyBlockType {
		return nil, fmt.Errorf("wrong type in %s: %s", path, p.Type)
	}

	key, err := x509.ParseECPrivateKey(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse key in %s: %v", path, err)
	}
	return key, nil
}

func loadCertificate(path string, ctx *renderContext) (*x509.Certificate, error) {
	crtS, err := getSecret("tls-self-signed-cert:"+path, ctx)
	if err != nil {
		return nil, err
	}

	crtBytes := []byte(crtS)
	if len(crtBytes) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}

	p, _ := pem.Decode(crtBytes)
	if p.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("wrong type in %s: %s", path, p.Type)
	}

	crt, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate in %s: %v", path, err)
	}

	return crt, nil
}
