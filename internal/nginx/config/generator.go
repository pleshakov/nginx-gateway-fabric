package config

import (
	"text/template"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state/dataplane"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Generator

// Generator generates NGINX configuration.
// This interface is used for testing purposes only.
type Generator interface {
	// Generate generates NGINX configuration from internal representation.
	Generate(configuration dataplane.Configuration) []byte
}

// GeneratorImpl is an implementation of Generator.
type GeneratorImpl struct {
	template *template.Template
}

// NewGeneratorImpl creates a new GeneratorImpl.
func NewGeneratorImpl() GeneratorImpl {
	t, err := template.New("nginx").Parse(mainTemplate)
	if err != nil {
		panic(err)
	}

	return GeneratorImpl{
		template: t,
	}
}

// executeFunc is a function that generates NGINX configuration from internal representation.
type executeFunc func(configuration dataplane.Configuration) []byte

// Generate generates NGINX configuration from internal representation.
// It is the responsibility of the caller to validate the configuration before calling this function.
// In case of invalid configuration, NGINX will fail to reload or could be configured with malicious configuration.
// To validate, use the validators from the validation package.
func (g GeneratorImpl) Generate(conf dataplane.Configuration) []byte {
	cfg := http.Config{
		Upstreams:    createUpstreams(conf.Upstreams),
		SplitClients: createSplitClients(conf.BackendGroups),
		Servers:      createServers(conf.HTTPServers, conf.SSLServers),
	}

	return execute(g.template, cfg)

}

func getExecuteFuncs() []executeFunc {
	return []executeFunc{
		executeUpstreams,
		executeSplitClients,
		executeServers,
	}
}
