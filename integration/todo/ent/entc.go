//go:build ignore

package main

import (
	"log"

	"entgo.io/contrib/entgql"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	cachegen "github.com/woocoos/entcache/gen"
)

func main() {
	nodeTpl := gen.MustParse(gen.NewTemplate("node").
		Funcs(entgql.TemplateFuncs).ParseFiles("./ent/template/node.tmpl"))
	ex, err := entgql.NewExtension(
		entgql.WithSchemaGenerator(),
		entgql.WithWhereFilters(true),
		entgql.WithConfigPath("./gqlgen.yml"),
		// Generate the filters to a separate schema
		// file and load it in the gqlgen.yml config.
		entgql.WithSchemaPath("./ent.graphql"),
		entgql.WithTemplates(append(entgql.AllTemplates, nodeTpl)...),
	)
	if err != nil {
		log.Fatalf("creating entgql extension: %v", err)
	}
	opts := []entc.Option{
		entc.Extensions(ex),
		cachegen.QueryCache(),
		//entc.TemplateDir("./ent/template"),
		entc.FeatureNames("intercept", "schema/snapshot"),
	}
	if err := entc.Generate("./ent/schema", &gen.Config{}, opts...); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
