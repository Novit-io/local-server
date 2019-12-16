package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"path"
	"reflect"

	yaml "gopkg.in/yaml.v2"

	"novit.nc/direktil/local-server/pkg/clustersconfig"
	"novit.nc/direktil/pkg/config"
)

type renderContext struct {
	Labels      map[string]string
	Annotations map[string]string

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
		mapMerge(vars, oVars)
	}

	return &renderContext{
		Labels:      mergeLabels(cluster.Labels, group.Labels, host.Labels),
		Annotations: mergeLabels(cluster.Annotations, group.Annotations, host.Annotations),

		Host:               host,
		Group:              group,
		Cluster:            cluster,
		Vars:               vars,
		ConfigTemplate:     cfg.ConfigTemplate(group.Config),
		StaticPodsTemplate: cfg.StaticPodsTemplate(group.StaticPods),

		clusterConfig: cfg,
	}, nil
}

func mergeLabels(sources ...map[string]string) map[string]string {
	ret := map[string]string{}

	for _, src := range sources {
		for k, v := range src {
			ret[k] = v
		}
	}

	return ret
}

func mapMerge(target, source map[string]interface{}) {
	for k, v := range source {
		target[k] = genericMerge(target[k], v)
	}
}

func genericMerge(target, source interface{}) (result interface{}) {
	srcV := reflect.ValueOf(source)
	tgtV := reflect.ValueOf(target)

	if srcV.Kind() == reflect.Map && tgtV.Kind() == reflect.Map {
		// XXX maybe more specific later
		result = map[interface{}]interface{}{}
		resultV := reflect.ValueOf(result)

		tgtIt := tgtV.MapRange()
		for tgtIt.Next() {
			sv := srcV.MapIndex(tgtIt.Key())
			if sv.Kind() == 0 {
				resultV.SetMapIndex(tgtIt.Key(), tgtIt.Value())
				continue
			}

			merged := genericMerge(tgtIt.Value().Interface(), sv.Interface())
			resultV.SetMapIndex(tgtIt.Key(), reflect.ValueOf(merged))
		}

		srcIt := srcV.MapRange()
		for srcIt.Next() {
			if resultV.MapIndex(srcIt.Key()).Kind() != 0 {
				continue // already done
			}

			resultV.SetMapIndex(srcIt.Key(), srcIt.Value())
		}

		return
	}

	return source
}

func (ctx *renderContext) Name() string {
	switch {
	case ctx.Host != nil:
		return "host:" + ctx.Host.Name
	case ctx.Group != nil:
		return "group:" + ctx.Group.Name
	case ctx.Cluster != nil:
		return "cluster:" + ctx.Cluster.Name
	default:
		return "unknown"
	}
}

func (ctx *renderContext) Config() string {
	if ctx.ConfigTemplate == nil {
		log.Fatalf("no such config: %q", ctx.Group.Config)
	}

	ctxName := ctx.Name()

	ctxMap := ctx.asMap()

	templateFuncs := ctx.templateFuncs(ctxMap)

	render := func(what string, t *clustersconfig.Template) (s string, err error) {
		buf := &bytes.Buffer{}
		err = t.Execute(ctxName, what, buf, ctxMap, templateFuncs)
		if err != nil {
			log.Printf("host %s: failed to render %s [%q]: %v", ctx.Host.Name, what, t.Name, err)
			return
		}

		s = buf.String()
		return
	}

	extraFuncs := ctx.templateFuncs(ctxMap)

	extraFuncs["static_pods"] = func() (string, error) {
		name := ctx.Group.StaticPods
		if len(name) == 0 {
			return "", fmt.Errorf("group %q has no static pods defined", ctx.Group.Name)
		}

		t := ctx.clusterConfig.StaticPodsTemplate(name)
		if t == nil {
			return "", fmt.Errorf("no static pods template named %q", name)
		}

		return render("static-pods", t)
	}

	extraFuncs["bootstrap_pods_files"] = func(dir string) (string, error) {
		namePods := renderBootstrapPods(ctx.Cluster)

		defs := make([]config.FileDef, 0)

		for _, namePod := range namePods {
			name := namePod.Namespace + "_" + namePod.Name

			ba, err := yaml.Marshal(namePod.Pod)
			if err != nil {
				return "", fmt.Errorf("bootstrap pod %s: failed to render: %v", name, err)
			}

			defs = append(defs, config.FileDef{
				Path:    path.Join(dir, name+".yaml"),
				Mode:    0640,
				Content: string(ba),
			})
		}

		ba, err := yaml.Marshal(defs)
		return string(ba), err
	}

	extraFuncs["machine_id"] = func() string {
		ba := sha1.Sum([]byte(ctx.Cluster.Name + "/" + ctx.Host.Name)) // TODO: check semantics of machine-id
		return hex.EncodeToString(ba[:])
	}

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err := ctx.ConfigTemplate.Execute(ctxName, "config", buf, ctxMap, extraFuncs); err != nil {
		log.Fatalf("failed to render config %q for host %q: %v", ctx.Group.Config, ctx.Host.Name, err)
	}

	return buf.String()
}

func (ctx *renderContext) StaticPods() (ba []byte, err error) {
	if ctx.StaticPodsTemplate == nil {
		log.Fatalf("no such static-pods: %q", ctx.Group.StaticPods)
	}

	ctxMap := ctx.asMap()

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err = ctx.StaticPodsTemplate.Execute(ctx.Name(), "static-pods", buf, ctxMap, ctx.templateFuncs(ctxMap)); err != nil {
		return
	}

	ba = buf.Bytes()
	return
}

func (ctx *renderContext) templateFuncs(ctxMap map[string]interface{}) map[string]interface{} {
	cluster := ctx.Cluster.Name

	getKeyCert := func(name, funcName string) (s string, err error) {
		req := ctx.clusterConfig.CSR(name)
		if req == nil {
			err = fmt.Errorf("no certificate request named %q", name)
			return
		}

		if req.CA == "" {
			err = fmt.Errorf("CA not defined in req %q", name)
			return
		}

		buf := &bytes.Buffer{}
		err = req.Execute(ctx.Name(), "req:"+name, buf, ctxMap, nil)
		if err != nil {
			return
		}

		key := name
		if req.PerHost {
			key += "/" + ctx.Host.Name
		}

		if funcName == "tls_dir" {
			// needs the dir name
			dir := "/etc/tls/" + name

			s = fmt.Sprintf("{{ %s %q %q %q %q %q %q %q }}", funcName,
				dir, cluster, req.CA, key, req.Profile, req.Label, buf.String())

		} else {
			s = fmt.Sprintf("{{ %s %q %q %q %q %q %q }}", funcName,
				cluster, req.CA, key, req.Profile, req.Label, buf.String())
		}
		return
	}

	funcs := clusterFuncs(ctx.Cluster)
	for k, v := range map[string]interface{}{
		"tls_key": func(name string) (string, error) {
			return getKeyCert(name, "tls_key")
		},

		"tls_crt": func(name string) (s string, err error) {
			return getKeyCert(name, "tls_crt")
		},

		"tls_dir": func(name string) (s string, err error) {
			return getKeyCert(name, "tls_dir")
		},

		"ssh_host_keys": func(dir string) (s string) {
			return fmt.Sprintf("{{ ssh_host_keys %q %q %q}}",
				dir, cluster, ctx.Host.Name)
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
				if host.Group == ctx.Host.Group {
					count++
				}
			}
			return
		},
	} {
		funcs[k] = v
	}
	return funcs
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
