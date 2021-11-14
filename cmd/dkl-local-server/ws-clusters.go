package main

import (
	"log"
	"sort"

	restful "github.com/emicklei/go-restful"

	"novit.nc/direktil/pkg/localconfig"
)

func wsListClusters(req *restful.Request, resp *restful.Response) {
	cfg := wsReadConfig(resp)
	if cfg == nil {
		return
	}

	clusterNames := make([]string, len(cfg.Clusters))
	for i, cluster := range cfg.Clusters {
		clusterNames[i] = cluster.Name
	}

	resp.WriteEntity(clusterNames)
}

func wsReadCluster(req *restful.Request, resp *restful.Response) (cluster *localconfig.Cluster) {
	clusterName := req.PathParameter("cluster-name")

	cfg := wsReadConfig(resp)
	if cfg == nil {
		return
	}

	cluster = cfg.Cluster(clusterName)
	if cluster == nil {
		wsNotFound(req, resp)
		return
	}

	return
}

func wsCluster(req *restful.Request, resp *restful.Response) {
	cluster := wsReadCluster(req, resp)
	if cluster == nil {
		return
	}

	resp.WriteEntity(cluster)
}

func wsClusterAddons(req *restful.Request, resp *restful.Response) {
	cluster := wsReadCluster(req, resp)
	if cluster == nil {
		return
	}

	if len(cluster.Addons) == 0 {
		log.Printf("cluster %q has no addons defined", cluster.Name)
		wsNotFound(req, resp)
		return
	}

	wsRender(resp, cluster.Addons, cluster)
}

func wsClusterPasswords(req *restful.Request, resp *restful.Response) {
	cluster := wsReadCluster(req, resp)
	if cluster == nil {
		return
	}

	resp.WriteEntity(secretData.Passwords(cluster.Name))
}

func wsClusterPassword(req *restful.Request, resp *restful.Response) {
	cluster := wsReadCluster(req, resp)
	if cluster == nil {
		return
	}

	name := req.PathParameter("password-name")

	resp.WriteEntity(secretData.Password(cluster.Name, name))
}

func wsClusterSetPassword(req *restful.Request, resp *restful.Response) {
	cluster := wsReadCluster(req, resp)
	if cluster == nil {
		return
	}

	name := req.PathParameter("password-name")

	var password string
	if err := req.ReadEntity(&password); err != nil {
		wsError(resp, err) // FIXME this is a BadRequest
		return
	}

	secretData.SetPassword(cluster.Name, name, password)

	if err := secretData.Save(); err != nil {
		wsError(resp, err)
		return
	}
}

func wsClusterToken(req *restful.Request, resp *restful.Response) {
	cluster := wsReadCluster(req, resp)
	if cluster == nil {
		return
	}

	name := req.PathParameter("token-name")

	token, err := secretData.Token(cluster.Name, name)
	if err != nil {
		wsError(resp, err)
		return
	}

	resp.WriteEntity(token)
}

func wsClusterBootstrapPods(req *restful.Request, resp *restful.Response) {
	cluster := wsReadCluster(req, resp)
	if cluster == nil {
		return
	}

	if len(cluster.BootstrapPods) == 0 {
		log.Printf("cluster %q has no bootstrap pods defined", cluster.Name)
		wsNotFound(req, resp)
		return
	}

	wsRender(resp, cluster.BootstrapPods, cluster)
}

func wsClusterCAs(req *restful.Request, resp *restful.Response) {
	cs := secretData.clusters[req.PathParameter("cluster-name")]
	if cs == nil {
		wsNotFound(req, resp)
		return
	}

	keys := make([]string, 0, len(cs.CAs))
	for k := range cs.CAs {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	resp.WriteJson(keys, restful.MIME_JSON)
}

func wsClusterCACert(req *restful.Request, resp *restful.Response) {
	cs := secretData.clusters[req.PathParameter("cluster-name")]
	if cs == nil {
		wsNotFound(req, resp)
		return
	}

	ca := cs.CAs[req.PathParameter("ca-name")]
	if ca == nil {
		wsNotFound(req, resp)
		return
	}

	resp.Write(ca.Cert)
}

func wsClusterSignedCert(req *restful.Request, resp *restful.Response) {
	cs := secretData.clusters[req.PathParameter("cluster-name")]
	if cs == nil {
		wsNotFound(req, resp)
		return
	}

	ca := cs.CAs[req.PathParameter("ca-name")]
	if ca == nil {
		wsNotFound(req, resp)
		return
	}

	name := req.QueryParameter("name")

	if name == "" {
		keys := make([]string, 0, len(ca.Signed))
		for k := range ca.Signed {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		resp.WriteJson(keys, restful.MIME_JSON)
		return
	}

	kc := ca.Signed[name]
	if kc == nil {
		wsNotFound(req, resp)
		return
	}

	resp.Write(kc.Cert)
}
