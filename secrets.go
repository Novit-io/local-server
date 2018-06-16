package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
	yaml "gopkg.in/yaml.v2"
)

var (
	secrets SecretBackend
)

type SecretData struct {
	clusters map[string]*ClusterSecrets
	changed  bool
	config   *config.Config
}

type ClusterSecrets struct {
	CAs map[string]*CA
}

type CA struct {
	Key  []byte
	Cert []byte

	Signed map[string]*KeyCert
}

type KeyCert struct {
	Key  []byte
	Cert []byte
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
		CAs: make(map[string]*CA),
	}
	sd.clusters[name] = cs
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

	kc, ok := ca.Signed[name]
	if ok {
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
		Key:  key,
		Cert: cert,
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

type SecretBackend interface {
	Get(ref string) (string, error)
	Set(ref, value string) error
}

type SecretsFile struct {
	Path string
}

func (sf *SecretsFile) readData() (map[string]string, error) {
	ba, err := ioutil.ReadFile(sf.Path)
	if err != nil {
		return nil, err
	}

	data := map[string]string{}
	yaml.Unmarshal(ba, &data)

	return data, nil
}

func (sf *SecretsFile) Get(ref string) (string, error) {
	data, err := sf.readData()

	if os.IsNotExist(err) {
		return "", nil

	} else if err != nil {
		log.Printf("secret file: failed to read: %v", err)
		return "", err
	}

	return data[ref], nil
}

func (sf *SecretsFile) Set(ref, value string) (err error) {
	data, err := sf.readData()

	if os.IsNotExist(err) {
		data = map[string]string{}

	} else if err != nil {
		log.Printf("secret file: failed to read: %v", err)
		return
	}

	data[ref] = value

	ba, err := yaml.Marshal(data)
	if err != nil {
		log.Printf("secret file: failed to encode: %v", err)
		return
	}

	os.Rename(sf.Path, sf.Path+".old")

	err = ioutil.WriteFile(sf.Path, ba, 0600)
	if err != nil {
		log.Printf("secret file: failed to write: %v", err)
		return
	}

	return
}

func getSecret(ref string, ctx *renderContext) (string, error) {
	fullRef := fmt.Sprintf("%s/%s", ctx.Cluster.Name, ref)

	v, err := secrets.Get(fullRef)

	if err != nil {
		return "", err
	}

	if v != "" {
		return v, nil
	}

	// no value, generate
	split := strings.SplitN(ref, ":", 2)
	kind, path := split[0], split[1]

	switch kind {
	case "tls-key":
		_, ba := PrivateKeyPEM()
		v = string(ba)

	case "tls-self-signed-cert":
		caKey, err := loadPrivateKey(path, ctx)
		if err != nil {
			return "", err
		}

		ba := SelfSignedCertificatePEM(5, caKey)
		v = string(ba)

	case "tls-host-cert":
		hostKey, err := loadPrivateKey(path, ctx)
		if err != nil {
			return "", err
		}

		ba, err := HostCertificatePEM(3, hostKey, ctx)
		if err != nil {
			return "", err
		}
		v = string(ba)

	default:
		return "", fmt.Errorf("unknown secret kind: %q", kind)
	}

	if v == "" {
		panic("value not generated?!")
	}

	if err := secrets.Set(fullRef, v); err != nil {
		return "", err
	}

	return v, nil
}
