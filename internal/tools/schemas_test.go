package tools

import (
	"encoding/json"
	"testing"
)

func TestToolSchemasAreValidJSON(t *testing.T) {
	schemas := map[string][]byte{
		"read":            schemaRead,
		"write":           schemaWrite,
		"bash":            schemaBash,
		"grep":            schemaGrep,
		"glob":            schemaGlob,
		"inspect":         schemaInspect,
		"web_fetch":       schemaWebFetch,
		"web_search":      schemaWebSearch,
		"task":            schemaTask,
		"activate_skill":  schemaActivateSkill,
		"question":        schemaQuestion,
		"delegate":        schemaDelegate,
		"brain_write":     schemaBrainWrite,
		"brain_read":      schemaBrainRead,
		"brain_list":      schemaBrainList,
		"patch":           schemaPatch,
	}
	for name, raw := range schemas {
		t.Run(name, func(t *testing.T) {
			if len(raw) == 0 {
				t.Fatalf("schema for %s is empty", name)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("schema for %s is not valid JSON: %v", name, err)
			}
			if doc["type"] != "object" {
				t.Errorf("schema for %s must be an object schema, got %v", name, doc["type"])
			}
			if _, ok := doc["properties"]; !ok {
				// brain_list has no properties, that's fine.
				if name != "brain_list" {
					t.Errorf("schema for %s missing properties", name)
				}
			}
		})
	}
}

func TestToolSpecsExposeSchemas(t *testing.T) {
	registry := NewRegistry() // registers read, glob, grep, bash, write, patch
	specs := registry.Specs(ToolContext{})
	byName := make(map[string]Spec, len(specs))
	for _, s := range specs {
		byName[s.Name] = s
	}
	for _, name := range []string{"read", "glob", "grep", "bash", "write", "patch"} {
		spec, ok := byName[name]
		if !ok {
			t.Errorf("expected tool %s registered", name)
			continue
		}
		if len(spec.InputSchema) == 0 {
			t.Errorf("expected %s to expose an InputSchema", name)
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(spec.InputSchema, &doc); err != nil {
			t.Errorf("%s InputSchema is not valid JSON: %v", name, err)
		}
	}
}

func TestSchemaRequiredFields(t *testing.T) {
	cases := []struct {
		name     string
		schema   []byte
		required []string
	}{
		{"read", schemaRead, []string{"path"}},
		{"write", schemaWrite, []string{"path", "content"}},
		{"bash", schemaBash, []string{"command"}},
		{"grep", schemaGrep, []string{"pattern"}},
		{"glob", schemaGlob, []string{"pattern"}},
		{"web_fetch", schemaWebFetch, []string{"url"}},
		{"web_search", schemaWebSearch, []string{"query"}},
		{"activate_skill", schemaActivateSkill, []string{"name"}},
		{"question", schemaQuestion, []string{"question"}},
		{"delegate", schemaDelegate, []string{"agent", "instruction"}},
	}
	for _, tc := range cases {
		var doc struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(tc.schema, &doc); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		for _, want := range tc.required {
			found := false
			for _, got := range doc.Required {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s schema missing required %q (got %v)", tc.name, want, doc.Required)
			}
		}
	}
}
