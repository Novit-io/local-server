package main

import (
	"strings"

	restful "github.com/emicklei/go-restful"
)

func adminAuth(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	tokenAuth(req, resp, chain, *adminToken)
}

func hostsAuth(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	tokenAuth(req, resp, chain, *hostsToken, *adminToken)
}

func tokenAuth(req *restful.Request, resp *restful.Response, chain *restful.FilterChain, allowedTokens ...string) {
	token := getToken(req)

	for _, allowedToken := range allowedTokens {
		if allowedToken == "" || token == allowedToken {
			chain.ProcessFilter(req, resp)
			return
		}
	}

	resp.WriteErrorString(401, "401: Not Authorized")
	return
}

func getToken(req *restful.Request) string {
	const bearerPrefix = "Bearer "

	token := req.HeaderParameter("Authorization")

	if !strings.HasPrefix(token, bearerPrefix) {
		return ""
	}

	return token[len(bearerPrefix):]
}
