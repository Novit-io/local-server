package main

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
)

type SecretData struct {
	clusters map[string]*ClusterSecrets
	changed  bool
	config   *config.Config
}

type ClusterSecrets struct {
	CAs    map[string]*CA
	Tokens map[string]string
}

type CA struct {
	Key  []byte
	Cert []byte

	Signed map[string]*KeyCert
}

type KeyCert struct {
	Key     []byte
	Cert    []byte
	ReqHash string
}

func loadSecretData(config *config.Config) (*SecretData, error) {
	sd := &SecretData{
		clusters: make(map[string]*ClusterSecrets),
		changed:  false,
		config:   config,
	}

	ba, err := ioutil.ReadFile(filepath.Join(*dataDir, "secret-data.json"))
	if err != nil {
		if os.IsNotExist(err) {
			sd.changed = true
			return sd, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(ba, &sd.clusters); err != nil {
		return nil, err
	}

	return sd, nil
}

func (sd *SecretData) Changed() bool {
	return sd.changed
}

func (sd *SecretData) Save() error {
	ba, err := json.Marshal(sd.clusters)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(*dataDir, "secret-data.json"), ba, 0600)
}

func (sd *SecretData) cluster(name string) (cs *ClusterSecrets) {
	cs, ok := sd.clusters[name]
	if ok {
		return
	}

	cs = &ClusterSecrets{
		CAs:    make(map[string]*CA),
		Tokens: make(map[string]string),
	}
	sd.clusters[name] = cs
	sd.changed = true
	return
}

func (sd *SecretData) Token(cluster, name string) (token string, err error) {
	cs := sd.cluster(cluster)

	token = cs.Tokens[name]
	if token != "" {
		return
	}

	b := make([]byte, 16)
	_, err = rand.Read(b)
	if err != nil {
		return
	}

	token = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)

	cs.Tokens[name] = token
	sd.changed = true
	return
}

func (sd *SecretData) CA(cluster, name string) (ca *CA, err error) {
	cs := sd.cluster(cluster)

	ca, ok := cs.CAs[name]
	if ok {
		return
	}

	req := &csr.CertificateRequest{
		CN: "Direktil Local Server",
		KeyRequest: &csr.BasicKeyRequest{
			A: "ecdsa",
			S: 521, // 256, 384, 521
		},
		Names: []csr.Name{
			{
				C: "NC",
				O: "novit.nc",
			},
		},
	}

	cert, _, key, err := initca.New(req)
	if err != nil {
		return
	}

	ca = &CA{
		Key:    key,
		Cert:   cert,
		Signed: make(map[string]*KeyCert),
	}

	cs.CAs[name] = ca
	sd.changed = true

	return
}

func (sd *SecretData) KeyCert(cluster, caName, name, profile, label string, req *csr.CertificateRequest) (kc *KeyCert, err error) {
	if req.CA != nil {
		err = errors.New("no CA section allowed here")
		return
	}

	ca, err := sd.CA(cluster, caName)
	if err != nil {
		return
	}

	rh := hash(req)
	kc, ok := ca.Signed[name]
	if ok && rh == kc.ReqHash {
		return
	}

	sgr, err := ca.Signer(sd.config.Signing)
	if err != nil {
		return
	}

	generator := &csr.Generator{Validator: func(_ *csr.CertificateRequest) error { return nil }}

	csr, key, err := generator.ProcessRequest(req)
	if err != nil {
		return
	}

	signReq := signer.SignRequest{
		Request: string(csr),
		Profile: profile,
		Label:   label,
	}

	cert, err := sgr.Sign(signReq)
	if err != nil {
		return
	}

	kc = &KeyCert{
		Key:     key,
		Cert:    cert,
		ReqHash: rh,
	}

	ca.Signed[name] = kc
	sd.changed = true

	return
}

func (ca *CA) Signer(policy *config.Signing) (result *local.Signer, err error) {
	caCert, err := helpers.ParseCertificatePEM(ca.Cert)
	if err != nil {
		return
	}

	caKey, err := helpers.ParsePrivateKeyPEM(ca.Key)
	if err != nil {
		return
	}

	return local.NewSigner(caKey, caCert, signer.DefaultSigAlgo(caKey), policy)
}
