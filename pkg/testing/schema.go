package testing

import (
	"fmt"
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/konveyor/analyzer-lsp/provider"
)

var t = &TestsFile{}
var cv = &CountBasedVerification{}
var lv = &LocationBasedVerification{}

func GenerateTestsSchema() (*openapi3.SchemaRef, error) {
	schemas := make(openapi3.Schemas)
	generator := openapi3gen.NewGenerator(
		openapi3gen.SchemaCustomizer(testsSchemaCustomizer))
	return generator.NewSchemaRefForValue(t, schemas)
}

func testsSchemaCustomizer(name string, t reflect.Type, tag reflect.StructTag, schema *openapi3.Schema) error {
	switch name {
	case "tests":
		if schema.Type.Is(openapi3.TypeObject) {
			schema.Required = append(schema.Required, "ruleID")
		}
	case "testCases":
		if schema.Type.Is(openapi3.TypeObject) {
			schema.Required = append(schema.Required, "name")
		}
	case "providers":
		if schema.Type.Is(openapi3.TypeObject) {
			schema.Required = append(schema.Required, "name")
			schema.Required = append(schema.Required, "dataPath")
		} else {
			schema.Nullable = true
		}
	case "hasIncidents":
		generator := openapi3gen.NewGenerator(
			openapi3gen.SchemaCustomizer(testsSchemaCustomizer))
		schemas := make(openapi3.Schemas)
		countBasedSchema, err := generator.NewSchemaRefForValue(cv, schemas)
		if err != nil {
			return err
		}
		locationBasedSchema, err := generator.NewSchemaRefForValue(lv, schemas)
		if err != nil {
			return err
		}
		// handle inline properties correctly
		delete(schema.Properties, "CountBased")
		delete(schema.Properties, "LocationBased")
		merge(schema.Properties, locationBasedSchema.Value.Properties)
		merge(schema.Properties, countBasedSchema.Value.Properties)
		schema.Nullable = true
	case "locations":
		if schema.Type.Is(openapi3.TypeObject) {
			schema.Required = append(schema.Required, "lineNumber")
			schema.Required = append(schema.Required, "fileURI")
		} else {
			schema.Nullable = true
		}
	case "atLeast", "atMost", "exactly", "hasTags":
		schema.Nullable = true
	case "mode":
		schema.Pattern = fmt.Sprintf("(%s|%s)", provider.FullAnalysisMode, provider.SourceOnlyAnalysisMode)
	}
	return nil
}

func merge(p1, p2 openapi3.Schemas) {
	for k, v := range p2 {
		p1[k] = v
	}
}
