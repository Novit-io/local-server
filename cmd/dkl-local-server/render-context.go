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

func newRenderContext(host *clustersconfig.Host, cfg *clustersconfig.Config) (ctx *renderContext, err error) {
	cluster := cfg.Cluster(host.Cluster)
	if cluster == nil {
		err = fmt.Errorf("no cluster named %q", host.Cluster)
		return
	}

	group := cfg.Group(host.Group)
	if group == nil {
		err = fmt.Errorf("no group named %q", host.Group)
		return
	}

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
	}, nil
}

func (ctx *renderContext) Config() (ba []byte, cfg *config.Config, err error) {
	if ctx.ConfigTemplate == nil {
		err = notFoundError{fmt.Sprintf("config %q", ctx.Group.Config)}
		return
	}

	ctxMap := ctx.asMap()

	secretData, err := ctx.secretData()
	if err != nil {
		return
	}

	templateFuncs := ctx.templateFuncs(secretData, ctxMap)

	render := func(what string, t *clustersconfig.Template) (s string, err error) {
		buf := &bytes.Buffer{}
		err = t.Execute(buf, ctxMap, templateFuncs)
		if err != nil {
			log.Printf("host %s: failed to render %s [%q]: %v", ctx.Host.Name, what, t.Name, err)
			return
		}

		s = buf.String()
		return
	}

	extraFuncs := ctx.templateFuncs(secretData, ctxMap)

	extraFuncs["static_pods"] = func(name string) (string, error) {
		t := ctx.clusterConfig.StaticPodsTemplate(name)
		if t == nil {
			return "", fmt.Errorf("no static pods template named %q", name)
		}

		return render("static pods", t)
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

func (ctx *renderContext) secretData() (data *SecretData, err error) {
	var sslCfg *cfsslconfig.Config

	if ctx.clusterConfig.SSLConfig == "" {
		sslCfg = &cfsslconfig.Config{}
	} else {
		sslCfg, err = cfsslconfig.LoadConfig([]byte(ctx.clusterConfig.SSLConfig))
		if err != nil {
			return
		}
	}

	data, err = loadSecretData(sslCfg)
	return
}

func (ctx *renderContext) StaticPods() (ba []byte, err error) {
	secretData, err := ctx.secretData()
	if err != nil {
		return
	}

	if ctx.StaticPodsTemplate == nil {
		err = notFoundError{fmt.Sprintf("static-pods %q", ctx.Group.StaticPods)}
		return
	}

	ctxMap := ctx.asMap()

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err = ctx.StaticPodsTemplate.Execute(buf, ctxMap, ctx.templateFuncs(secretData, ctxMap)); err != nil {
		return
	}

	if secretData.Changed() {
		err = secretData.Save()
		if err != nil {
			return
		}
	}

	ba = buf.Bytes()
	return
}

func (ctx *renderContext) templateFuncs(secretData *SecretData, ctxMap map[string]interface{}) map[string]interface{} {
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

	return map[string]interface{}{
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

			dir := "/" + path.Join("etc", "tls-ca", name)

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

			dir := "/" + path.Join("etc", "tls", name)

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

		"hosts_of_group": func() (hosts []interface{}) {
			hosts = make([]interface{}, 0)

			for _, host := range ctx.clusterConfig.Hosts {
				if host.Group != ctx.Host.Group {
					continue
				}

				hosts = append(hosts, asMap(host))
			}

			return hosts
		},

		"hosts_of_group_count": func() (count int) {
			for _, host := range ctx.clusterConfig.Hosts {
				if host.Group != ctx.Host.Group {
					continue
				}

				count++
			}
			return
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

func (ctx *renderContext) asMap() map[string]interface{} {
	result := asMap(ctx)

	// also expand cluster:
	cluster := result["cluster"].(map[interface{}]interface{})
	cluster["kubernetes_svc_ip"] = ctx.Cluster.KubernetesSvcIP().String()
	cluster["dns_svc_ip"] = ctx.Cluster.DNSSvcIP().String()

	return result
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