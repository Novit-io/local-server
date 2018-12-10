package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"

	cfsslconfig "github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/golang/go/src/pkg/html/template"
	yaml "gopkg.in/yaml.v2"

	"novit.nc/direktil/pkg/config"
	"novit.nc/direktil/pkg/localconfig"
)

type renderContext struct {
	Host      *localconfig.Host
	SSLConfig string
}

func renderCtx(w http.ResponseWriter, r *http.Request, ctx *renderContext, what string,
	create func(out io.Writer, ctx *renderContext) error) error {
	log.Printf("sending %s for %q", what, ctx.Host.Name)

	tag, err := ctx.Tag()
	if err != nil {
		return err
	}

	// get it or create it
	content, meta, err := casStore.GetOrCreate(tag, what, func(out io.Writer) error {
		log.Printf("building %s for %q", what, ctx.Host.Name)
		return create(out, ctx)
	})

	if err != nil {
		return err
	}

	// serve it
	http.ServeContent(w, r, what, meta.ModTime(), content)
	return nil
}

func newRenderContext(host *localconfig.Host, cfg *localconfig.Config) (ctx *renderContext, err error) {
	return &renderContext{
		SSLConfig: cfg.SSLConfig,
		Host:      host,
	}, nil
}

func (ctx *renderContext) Config() (ba []byte, cfg *config.Config, err error) {
	secretData, err := ctx.secretData()
	if err != nil {
		return
	}

	tmpl, err := template.New(ctx.Host.Name + "/config").
		Funcs(ctx.templateFuncs(secretData)).
		Parse(ctx.Host.Config)

	if err != nil {
		return
	}

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err = tmpl.Execute(buf, nil); err != nil {
		return
	}

	if secretData.Changed() {
		err = secretData.Save()
		if err != nil {
			return
		}
	}

	ba = buf.Bytes()

	cfg = &config.Config{}

	if err = yaml.Unmarshal(buf.Bytes(), cfg); err != nil {
		return
	}

	return
}

func (ctx *renderContext) secretData() (data *SecretData, err error) {
	var sslCfg *cfsslconfig.Config

	if len(ctx.SSLConfig) == 0 {
		sslCfg = &cfsslconfig.Config{}
	} else {
		sslCfg, err = cfsslconfig.LoadConfig([]byte(ctx.SSLConfig))
		if err != nil {
			return
		}
	}

	data, err = loadSecretData(sslCfg)
	return
}

func (ctx *renderContext) templateFuncs(secretData *SecretData) map[string]interface{} {
	getKeyCert := func(cluster, caName, name, profile, label, reqJson string) (kc *KeyCert, err error) {
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

	asYaml := func(v interface{}) (string, error) {
		ba, err := yaml.Marshal(v)
		if err != nil {
			return "", err
		}

		return string(ba), nil
	}

	return map[string]interface{}{
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

		"tls_dir": func(cluster, caName, name, profile, label, reqJson string) (s string, err error) {
			ca, err := secretData.CA(cluster, caName)
			if err != nil {
				return
			}

			kc, err := getKeyCert(cluster, caName, name, profile, label, reqJson)
			if err != nil {
				return
			}

			dir := "/etc/tls/" + name

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
	}
}

func (ctx *renderContext) distFilePath(path ...string) string {
	return filepath.Join(append([]string{*dataDir, "dist"}, path...)...)
}

func (ctx *renderContext) Tag() (string, error) {
	h := sha256.New()

	_, cfg, err := ctx.Config()
	if err != nil {
		return "", err
	}

	enc := yaml.NewEncoder(h)

	for _, o := range []interface{}{cfg, ctx} {
		if err := enc.Encode(o); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func asMap(v interface{}) map[string]interface{} {
	ba, err := yaml.Marshal(v)
	if err != nil {
		panic(err) // shouldn't happen
	}

	result := make(map[string]interface{})

	if err := yaml.Unmarshal(ba, result); err != nil {
		panic(err) // shouldn't happen
	}

	return result
}
