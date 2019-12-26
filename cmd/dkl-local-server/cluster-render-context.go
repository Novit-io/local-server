package main

import (
	"encoding/json"
	"fmt"
	"log"
	"path"

	"github.com/cloudflare/cfssl/csr"
	yaml "gopkg.in/yaml.v2"
	"novit.nc/direktil/pkg/config"
)

var templateFuncs = map[string]interface{}{
	"password": func(cluster, name string) (password string, err error) {
		password = secretData.Password(cluster, name)
		if len(password) == 0 {
			err = fmt.Errorf("password %q not defined for cluster %q", name, cluster)
		}
		return
	},

	"token": func(cluster, name string) (s string, err error) {
		return secretData.Token(cluster, name)
	},

	"ca_key": func(cluster, name string) (s string, err error) {
		ca, err := secretData.CA(cluster, name)
		if err != nil {
			return
		}

		s = string(ca.Key)
		return
	},

	"ca_crt": func(cluster, name string) (s string, err error) {
		ca, err := secretData.CA(cluster, name)
		if err != nil {
			return
		}

		s = string(ca.Cert)
		return
	},

	"ca_dir": func(cluster, name string) (s string, err error) {
		ca, err := secretData.CA(cluster, name)
		if err != nil {
			return
		}

		dir := "/etc/tls-ca/" + name

		return asYaml([]config.FileDef{
			{
				Path:    path.Join(dir, "ca.crt"),
				Mode:    0644,
				Content: string(ca.Cert),
			},
			{
				Path:    path.Join(dir, "ca.key"),
				Mode:    0600,
				Content: string(ca.Key),
			},
		})
	},

	"tls_key": func(cluster, caName, name, profile, label, reqJson string) (s string, err error) {
		kc, err := getKeyCert(cluster, caName, name, profile, label, reqJson)
		if err != nil {
			return
		}

		s = string(kc.Key)
		return
	},

	"tls_crt": func(cluster, caName, name, profile, label, reqJson string) (s string, err error) {
		kc, err := getKeyCert(cluster, caName, name, profile, label, reqJson)
		if err != nil {
			return
		}

		s = string(kc.Cert)
		return
	},

	"tls_dir": func(dir, cluster, caName, name, profile, label, reqJson string) (s string, err error) {
		ca, err := secretData.CA(cluster, caName)
		if err != nil {
			return
		}

		kc, err := getKeyCert(cluster, caName, name, profile, label, reqJson)
		if err != nil {
			return
		}

		return asYaml([]config.FileDef{
			{
				Path:    path.Join(dir, "ca.crt"),
				Mode:    0644,
				Content: string(ca.Cert),
			},
			{
				Path:    path.Join(dir, "tls.crt"),
				Mode:    0644,
				Content: string(kc.Cert),
			},
			{
				Path:    path.Join(dir, "tls.key"),
				Mode:    0600,
				Content: string(kc.Key),
			},
		})
	},

	"ssh_host_keys": func(dir, cluster, host string) (s string, err error) {
		pairs, err := secretData.SSHKeyPairs(cluster, host)
		if err != nil {
			return
		}

		files := make([]config.FileDef, 0, len(pairs)*2)

		for _, pair := range pairs {
			basePath := path.Join(dir, "ssh_host_"+pair.Type+"_key")
			files = append(files, []config.FileDef{
				{
					Path:    basePath,
					Mode:    0600,
					Content: pair.Private,
				},
				{
					Path:    basePath + ".pub",
					Mode:    0644,
					Content: pair.Public,
				},
			}...)
		}

		return asYaml(files)
	},
}

func getKeyCert(cluster, caName, name, profile, label, reqJson string) (kc *KeyCert, err error) {
	certReq := &csr.CertificateRequest{
		KeyRequest: csr.NewBasicKeyRequest(),
	}

	err = json.Unmarshal([]byte(reqJson), certReq)
	if err != nil {
		log.Print("CSR unmarshal failed on: ", reqJson)
		return
	}

	return secretData.KeyCert(cluster, caName, name, profile, label, certReq)
}

func asYaml(v interface{}) (string, error) {
	ba, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(ba), nil
}
