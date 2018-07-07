package main

// Kubernetes' compatible ConfigMap
type configMap struct {
	APIVersion string `yaml:"apiVersion"` // v1
	Kind       string
	Metadata   metadata
	Data       map[string]string
}

type metadata struct {
	Namespace string
	Name      string
}

func newConfigMap(name string) *configMap {
	return &configMap{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Metadata: metadata{
			Namespace: "kube-system",
			Name:      name,
		},
		Data: make(map[string]string),
	}
}
