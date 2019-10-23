package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"path"

	yaml "gopkg.in/yaml.v2"

	"novit.nc/direktil/local-server/pkg/clustersconfig"
)

func clusterFuncs(clusterSpec *clustersconfig.Cluster) map[string]interface{} {
	cluster := clusterSpec.Name

	return map[string]interface{}{
		"password": func(name string) (s string) {
			return fmt.Sprintf("{{ password %q %q }}", cluster, name)
		},

		"token": func(name string) (s string) {
			return fmt.Sprintf("{{ token %q %q }}", cluster, name)
		},

		"ca_key": func(name string) (s string, err error) {
			// TODO check CA exists
			// ?ctx.clusterConfig.CA(name)
			return fmt.Sprintf("{{ ca_key %q %q }}", cluster, name), nil
		},

		"ca_crt": func(name string) (s string, err error) {
			// TODO check CA exists
			return fmt.Sprintf("{{ ca_crt %q %q }}", cluster, name), nil
		},

		"ca_dir": func(name string) (s string, err error) {
			return fmt.Sprintf("{{ ca_dir %q %q }}", cluster, name), nil
		},

		"hosts_by_group": func(group string) (hosts []interface{}) {
			for _, host := range src.Hosts {
				if host.Group == group {
					hosts = append(hosts, asMap(host))
				}
			}

			if len(hosts) == 0 {
				log.Fatalf("no hosts in group %q", group)
			}

			return
		},
	}
}

func renderClusterTemplates(cluster *clustersconfig.Cluster, setName string,
	templates []*clustersconfig.Template) []byte {
	clusterAsMap := asMap(cluster)
	clusterAsMap["kubernetes_svc_ip"] = cluster.KubernetesSvcIP().String()
	clusterAsMap["dns_svc_ip"] = cluster.DNSSvcIP().String()

	funcs := clusterFuncs(cluster)

	log.Print("rendering cluster templates in ", setName, " with ", clusterAsMap)

	buf := &bytes.Buffer{}

	contextName := "cluster:" + cluster.Name

	for _, t := range templates {
		log.Print("- template: ", setName, ": ", t.Name)
		fmt.Fprintf(buf, "---\n# %s: %s\n", setName, t.Name)
		err := t.Execute(contextName, path.Join(setName, t.Name), buf, clusterAsMap, funcs)

		if err != nil {
			log.Fatalf("cluster %q: %s: failed to render %q: %v",
				cluster.Name, setName, t.Name, err)
		}

		fmt.Fprintln(buf)
	}

	return buf.Bytes()
}

func renderAddons(cluster *clustersconfig.Cluster) string {
	if len(cluster.Addons) == 0 {
		return ""
	}

	addons := src.Addons[cluster.Addons]
	if addons == nil {
		log.Fatalf("cluster %q: no addons with name %q", cluster.Name, cluster.Addons)
	}

	return string(renderClusterTemplates(cluster, "addons", addons))
}

type namePod struct {
	Namespace string
	Name      string
	Pod       map[string]interface{}
}

func renderBootstrapPods(cluster *clustersconfig.Cluster) (pods []namePod) {
	if cluster.BootstrapPods == "" {
		return nil
	}

	bootstrapPods := src.BootstrapPods[cluster.BootstrapPods]
	if bootstrapPods == nil {
		log.Fatalf("no bootstrap pods template named %q", cluster.BootstrapPods)
	}

	// render bootstrap pods
	parts := bytes.Split(renderClusterTemplates(cluster, "bootstrap-pods", bootstrapPods), []byte("\n---\n"))
	for _, part := range parts {
		buf := bytes.NewBuffer(part)
		dec := yaml.NewDecoder(buf)

		for n := 0; ; n++ {
			str := buf.String()

			podMap := map[string]interface{}{}
			err := dec.Decode(podMap)

			if err == io.EOF {
				break
			} else if err != nil {
				log.Fatalf("bootstrap pod %d: failed to parse: %v\n%s", n, err, str)
			}

			if len(podMap) == 0 {
				continue
			}

			if podMap["metadata"] == nil {
				log.Fatalf("bootstrap pod %d: no metadata\n%s", n, buf.String())
			}

			md := podMap["metadata"].(map[interface{}]interface{})

			namespace := md["namespace"].(string)
			name := md["name"].(string)

			pods = append(pods, namePod{namespace, name, podMap})
		}
	}

	return
}

func renderBootstrapPodsDS(cluster *clustersconfig.Cluster) string {
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	for _, namePod := range renderBootstrapPods(cluster) {
		pod := namePod.Pod

		md := pod["metadata"].(map[interface{}]interface{})
		labels := md["labels"]

		if labels == nil {
			labels = map[string]interface{}{
				"app": namePod.Name,
			}
			md["labels"] = labels
		}

		ann := md["annotations"]
		annotations := map[interface{}]interface{}{}
		if ann != nil {
			annotations = ann.(map[interface{}]interface{})
		}
		annotations["node.kubernetes.io/bootstrap-checkpoint"] = "true"

		md["annotations"] = annotations

		delete(md, "name")
		delete(md, "namespace")

		err := enc.Encode(map[string]interface{}{
			"apiVersion": "extensions/v1beta1",
			"kind":       "DaemonSet",
			"metadata": map[string]interface{}{
				"namespace": namePod.Namespace,
				"name":      namePod.Name,
				"labels":    labels,
			},
			"spec": map[string]interface{}{
				"minReadySeconds": 60,
				"selector": map[string]interface{}{
					"matchLabels": labels,
				},
				"template": map[string]interface{}{
					"metadata": pod["metadata"],
					"spec":     pod["spec"],
				},
			},
		})

		if err != nil {
			panic(err)
		}
	}
	return buf.String()
}
