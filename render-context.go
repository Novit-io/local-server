package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"
	"path/filepath"

	cfsslconfig "github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	yaml "gopkg.in/yaml.v2"

	"novit.nc/direktil/pkg/clustersconfig"
	"novit.nc/direktil/pkg/config"
)

type renderContext struct {
	Host               *clustersconfig.Host
	Group              *clustersconfig.Group
	Cluster            *clustersconfig.Cluster
	Vars               map[string]interface{}
	ConfigTemplate     *clustersconfig.Template
	StaticPodsTemplate *clustersconfig.Template

	clusterConfig *clustersconfig.Config
}

func newRenderContext(host *clustersconfig.Host, cfg *clustersconfig.Config) *renderContext {
	group := cfg.Group(host.Group)
	cluster := cfg.Cluster(host.Cluster)

	vars := make(map[string]interface{})

	for _, oVars := range []map[string]interface{}{
		cluster.Vars,
		group.Vars,
		host.Vars,
	} {
		for k, v := range oVars {
			vars[k] = v
		}
	}

	return &renderContext{
		Host:               host,
		Group:              group,
		Cluster:            cluster,
		Vars:               vars,
		ConfigTemplate:     cfg.ConfigTemplate(group.Config),
		StaticPodsTemplate: cfg.StaticPodsTemplate(group.StaticPods),

		clusterConfig: cfg,
	}
}

func (ctx *renderContext) Config() (ba []byte, cfg *config.Config, err error) {
	if ctx.ConfigTemplate == nil {
		err = notFoundError{fmt.Sprintf("config %q", ctx.Group.Config)}
		return
	}

	ctxMap := ctx.asMap()

	sslCfg, err := cfsslconfig.LoadConfig([]byte(ctx.clusterConfig.SSLConfig))
	if err != nil {
		return
	}

	secretData, err := loadSecretData(sslCfg)
	if err != nil {
		return
	}

	cluster := ctx.Cluster.Name

	getKeyCert := func(name string) (kc *KeyCert, err error) {
		req := ctx.clusterConfig.CSR(name)
		if req == nil {
			err = errors.New("no such certificate request")
			return
		}

		if req.CA == "" {
			err = errors.New("CA not defined")
			return
		}

		buf := &bytes.Buffer{}
		err = req.Execute(buf, ctxMap, nil)
		if err != nil {
			return
		}

		certReq := &csr.CertificateRequest{
			KeyRequest: csr.NewBasicKeyRequest(),
		}

		err = json.Unmarshal(buf.Bytes(), certReq)
		if err != nil {
			log.Print("unmarshal failed on: ", buf)
			return
		}

		if req.PerHost {
			name = name + "/" + ctx.Host.Name
		}

		return secretData.KeyCert(cluster, req.CA, name, req.Profile, req.Label, certReq)
	}

	asYaml := func(v interface{}) (string, error) {
		ba, err := yaml.Marshal(v)
		if err != nil {
			return "", err
		}

		return string(ba), nil
	}

	extraFuncs := map[string]interface{}{
		"static_pods": func(name string) (string, error) {
			t := ctx.clusterConfig.StaticPodsTemplate(name)
			if t == nil {
				return "", fmt.Errorf("no static pods template named %q", name)
			}

			buf := &bytes.Buffer{}
			err := t.Execute(buf, ctxMap, nil)
			if err != nil {
				log.Printf("host %s: failed to render static pods: %v", ctx.Host.Name, err)
				return "", err
			}

			return buf.String(), nil
		},

		"token": func(name string) (s string, err error) {
			return secretData.Token(cluster, name)
		},

		"ca_key": func(name string) (s string, err error) {
			ca, err := secretData.CA(cluster, name)
			if err != nil {
				return
			}

			s = string(ca.Key)
			return
		},

		"ca_crt": func(name string) (s string, err error) {
			ca, err := secretData.CA(cluster, name)
			if err != nil {
				return
			}

			s = string(ca.Cert)
			return
		},

		"ca_dir": func(name string) (s string, err error) {
			ca, err := secretData.CA(cluster, name)
			if err != nil {
				return
			}

			dir := path.Join("etc", "tls-ca", name)

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

		"tls_key": func(name string) (s string, err error) {
			kc, err := getKeyCert(name)
			if err != nil {
				return
			}

			s = string(kc.Key)
			return
		},

		"tls_crt": func(name string) (s string, err error) {
			kc, err := getKeyCert(name)
			if err != nil {
				return
			}

			s = string(kc.Cert)
			return
		},

		"tls_dir": func(name string) (s string, err error) {
			csr := ctx.clusterConfig.CSR(name)
			if csr == nil {
				err = fmt.Errorf("no CSR named %q", name)
				return
			}

			ca, err := secretData.CA(cluster, csr.CA)
			if err != nil {
				return
			}

			kc, err := getKeyCert(name)
			if err != nil {
				return
			}

			dir := path.Join("etc", "tls", name)

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

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err = ctx.ConfigTemplate.Execute(buf, ctxMap, extraFuncs); err != nil {
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

func (ctx *renderContext) StaticPods() (ba []byte, err error) {
	if ctx.StaticPodsTemplate == nil {
		err = notFoundError{fmt.Sprintf("static-pods %q", ctx.Group.StaticPods)}
		return
	}

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err = ctx.StaticPodsTemplate.Execute(buf, ctx.asMap(), nil); err != nil {
		return
	}

	ba = buf.Bytes()
	return
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

func (ctx *renderContext) asMap() map[string]interface{} {
	ba, err := yaml.Marshal(ctx)
	if err != nil {
		panic(err) // shouldn't happen
	}

	result := make(map[string]interface{})

	if err := yaml.Unmarshal(ba, result); err != nil {
		panic(err)
	}

	// also expand cluster:
	cluster := result["cluster"].(map[interface{}]interface{})
	cluster["kubernetes_svc_ip"] = ctx.Cluster.KubernetesSvcIP().String()
	cluster["dns_svc_ip"] = ctx.Cluster.DNSSvcIP().String()

	return result
}
