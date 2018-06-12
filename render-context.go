package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"

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

	extraFuncs := map[string]interface{}{
		"static_pods": func(name string) (string, error) {
			t := ctx.clusterConfig.StaticPodsTemplate(name)
			if t == nil {
				return "", nil
			}

			buf := &bytes.Buffer{}
			err := t.Execute(buf, ctx, nil)
			if err != nil {
				log.Printf("host %s: failed to render static pods: %v", ctx.Host.Name, err)
				return "", err
			}

			return buf.String(), nil
		},
	}

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err = ctx.ConfigTemplate.Execute(buf, ctx, extraFuncs); err != nil {
		return
	}

	ba = buf.Bytes()

	cfg = &config.Config{}

	if err = yaml.Unmarshal(buf.Bytes(), cfg); err != nil {
		return
	}

	// bind secrets in config
	for idx, file := range cfg.Files {
		if file.Secret == "" {
			continue
		}

		v, err2 := getSecret(file.Secret, ctx)
		if err2 != nil {
			err = err2
			return
		}

		cfg.Files[idx].Content = v
	}

	return
}

func (ctx *renderContext) StaticPods() (ba []byte, err error) {
	if ctx.StaticPodsTemplate == nil {
		err = notFoundError{fmt.Sprintf("static-pods %q", ctx.Group.StaticPods)}
		return
	}

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err = ctx.StaticPodsTemplate.Execute(buf, ctx, nil); err != nil {
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
