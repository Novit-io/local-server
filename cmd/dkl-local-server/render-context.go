package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"text/template"

	cfsslconfig "github.com/cloudflare/cfssl/config"
	restful "github.com/emicklei/go-restful"
	yaml "gopkg.in/yaml.v2"

	"novit.nc/direktil/pkg/config"
	"novit.nc/direktil/pkg/localconfig"
)

var cmdlineParam = restful.QueryParameter("cmdline", "Linux kernel cmdline addition")

type renderContext struct {
	Host      *localconfig.Host
	SSLConfig string

	// Linux kernel extra cmdline
	CmdLine string `yaml:"-"`
}

func renderCtx(w http.ResponseWriter, r *http.Request, ctx *renderContext, what string,
	create func(out io.Writer, ctx *renderContext) error) error {

	tag, err := ctx.Tag()
	if err != nil {
		return err
	}

	ctx.CmdLine = r.URL.Query().Get(cmdlineParam.Data().Name)

	if ctx.CmdLine != "" {
		what = what + "?cmdline=" + url.QueryEscape(ctx.CmdLine)
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
	log.Printf("sending %s for %q", what, ctx.Host.Name)
	http.ServeContent(w, r, what, meta.ModTime(), content)
	return nil
}

var prevSSLConfig = "-"

func newRenderContext(host *localconfig.Host, cfg *localconfig.Config) (ctx *renderContext, err error) {
	if prevSSLConfig != cfg.SSLConfig {
		var sslCfg *cfsslconfig.Config

		if len(cfg.SSLConfig) == 0 {
			sslCfg = &cfsslconfig.Config{}
		} else {
			sslCfg, err = cfsslconfig.LoadConfig([]byte(cfg.SSLConfig))
			if err != nil {
				return
			}
		}

		err = loadSecretData(sslCfg)
		if err != nil {
			return
		}

		prevSSLConfig = cfg.SSLConfig
	}

	return &renderContext{
		SSLConfig: cfg.SSLConfig,
		Host:      host,
	}, nil
}

func (ctx *renderContext) Config() (ba []byte, cfg *config.Config, err error) {
	tmpl, err := template.New(ctx.Host.Name + "/config").
		Funcs(templateFuncs).
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
