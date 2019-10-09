package main

import (
	"bytes"
	"fmt"
	"io"
	"log"

	yaml "gopkg.in/yaml.v2"
	"novit.nc/direktil/local-server/pkg/clustersconfig"
)

func renderClusterTemplates(cluster *clustersconfig.Cluster, setName string,
	templates []*clustersconfig.Template) []byte {
	clusterAsMap := asMap(cluster)
	clusterAsMap["kubernetes_svc_ip"] = cluster.KubernetesSvcIP().String()
	clusterAsMap["dns_svc_ip"] = cluster.DNSSvcIP().String()

	buf := &bytes.Buffer{}

	for _, t := range templates {
		fmt.Fprintf(buf, "---\n# %s: %s\n", setName, t.Name)
		err := t.Execute(buf, clusterAsMap, nil)

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
	buf := bytes.NewBuffer(renderClusterTemplates(cluster, "bootstrap pods", bootstrapPods))
	dec := yaml.NewDecoder(buf)

	for n := 0; ; n++ {
		podMap := map[string]interface{}{}
		err := dec.Decode(podMap)

		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatalf("bootstrap pod %d: failed to parse: %v\n%s", n, err, buf.String())
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

	return
}

func renderBootstrapPodsDS(cluster *clustersconfig.Cluster) string {
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	for _, namePod := range renderBootstrapPods(cluster) {
		pod := namePod.Pod

		md := pod["metadata"].(map[interface{}]interface{})
		labels := md["labels"]

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
				"template": pod,
			},
		})

		if err != nil {
			panic(err)
		}
	}
	return buf.String()
}
