package gen

import (
	"embed"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

var (
	//go:embed template/*
	_templates embed.FS
)

// QueryCache returns an entc.Option that generates the cached Get. It overrides the default client.tmpl
func QueryCache() entc.Option {
	return func(c *gen.Config) error {
		c.Templates = append(c.Templates, gen.MustParse(gen.NewTemplate("client").
			ParseFS(_templates, "template/client.tmpl")))
		return nil
	}
}
