package main

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/cespare/xxhash"
	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var (
	secretData *SecretData
	DontSave   = false
)

type SecretData struct {
	l sync.Mutex

	prevHash uint64

	clusters map[string]*ClusterSecrets
	changed  bool
	config   *config.Config
}

type ClusterSecrets struct {
	CAs         map[string]*CA
	Tokens      map[string]string
	Passwords   map[string]string
	SSHKeyPairs map[string][]SSHKeyPair
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

func secretDataPath() string {
	return filepath.Join(*dataDir, "secret-data.json")
}

func loadSecretData(config *config.Config) (err error) {
	log.Info("Loading secret data")

	sd := &SecretData{
		clusters: make(map[string]*ClusterSecrets),
		changed:  false,
		config:   config,
	}

	ba, err := ioutil.ReadFile(secretDataPath())
	if err != nil {
		if os.IsNotExist(err) {
			sd.changed = true
			err = nil
			secretData = sd
			return
		}
		return
	}

	if err = json.Unmarshal(ba, &sd.clusters); err != nil {
		return
	}

	sd.prevHash = xxhash.Sum64(ba)

	secretData = sd
	return
}

func (sd *SecretData) Changed() bool {
	return sd.changed
}

func (sd *SecretData) Save() (err error) {
	if DontSave {
		return
	}

	sd.l.Lock()
	defer sd.l.Unlock()

	ba, err := json.Marshal(sd.clusters)
	if err != nil {
		return
	}

	h := xxhash.Sum64(ba)
	if h == sd.prevHash {
		return
	}

	log.Info("Saving secret data")
	err = ioutil.WriteFile(secretDataPath(), ba, 0600)

	if err == nil {
		sd.prevHash = h
	}

	return
}

func newClusterSecrets() *ClusterSecrets {
	return &ClusterSecrets{
		CAs:       make(map[string]*CA),
		Tokens:    make(map[string]string),
		Passwords: make(map[string]string),
	}
}

func (sd *SecretData) cluster(name string) (cs *ClusterSecrets) {
	cs, ok := sd.clusters[name]
	if ok {
		return
	}

	sd.l.Lock()
	defer sd.l.Unlock()

	log.Info("secret-data: new cluster: ", name)

	cs = newClusterSecrets()
	sd.clusters[name] = cs
	sd.changed = true
	return
}

func (sd *SecretData) Passwords(cluster string) (passwords []string) {
	cs := sd.cluster(cluster)

	passwords = make([]string, 0, len(cs.Passwords))
	for name := range cs.Passwords {
		passwords = append(passwords, name)
	}

	sort.Strings(passwords)

	return
}

func (sd *SecretData) Password(cluster, name string) (password string) {
	cs := sd.cluster(cluster)

	if cs.Passwords == nil {
		cs.Passwords = make(map[string]string)
	}

	password = cs.Passwords[name]
	return
}

func (sd *SecretData) SetPassword(cluster, name, password string) {
	cs := sd.cluster(cluster)

	if cs.Passwords == nil {
		cs.Passwords = make(map[string]string)
	}

	cs.Passwords[name] = password
	sd.changed = true
}

func (sd *SecretData) Token(cluster, name string) (token string, err error) {
	cs := sd.cluster(cluster)

	token = cs.Tokens[name]
	if token != "" {
		return
	}

	sd.l.Lock()
	defer sd.l.Unlock()

	log.Info("secret-data: new token in cluster ", cluster, ": ", name)

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

	sd.l.Lock()
	defer sd.l.Unlock()

	log.Info("secret-data: new CA in cluster ", cluster, ": ", name)

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
	for idx, host := range req.Hosts {
		if ip := net.ParseIP(host); ip != nil {
			// valid IP (v4 or v6)
			continue
		}

		if host == "*" {
			continue
		}

		if errs := validation.IsDNS1123Subdomain(host); len(errs) == 0 {
			continue
		}
		if errs := validation.IsWildcardDNS1123Subdomain(host); len(errs) == 0 {
			continue
		}

		path := field.NewPath(cluster, name, "hosts").Index(idx)
		return nil, fmt.Errorf("%v: %q is not an IP or FQDN", path, host)
	}

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
	} else if ok {
		log.Infof("secret-data: cluster %s: CA %s: CSR changed for %s: hash=%q previous=%q",
			cluster, caName, name, rh, kc.ReqHash)
	} else {
		log.Infof("secret-data: cluster %s: CA %s: new CSR for %s", cluster, caName, name)
	}

	sd.l.Lock()
	defer sd.l.Unlock()

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
