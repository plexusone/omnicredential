package schema

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestEmbeddedSchemasAreCurrent fails when the types changed and the
// schemas were not regenerated with go generate ./schema.
func TestEmbeddedSchemasAreCurrent(t *testing.T) {
	reg, ev, err := Generate("..")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reg, Registry()) {
		t.Error("registry.schema.json is stale: run go generate ./schema")
	}
	if !bytes.Equal(ev, Events()) {
		t.Error("events.schema.json is stale: run go generate ./schema")
	}
}

func TestSchemaContent(t *testing.T) {
	var s struct {
		ID   string `json:"$id"`
		Defs map[string]struct {
			Description string   `json:"description"`
			Required    []string `json:"required"`
			Properties  map[string]struct {
				Description string   `json:"description"`
				Pattern     string   `json:"pattern"`
				Enum        []string `json:"enum"`
				Examples    []string `json:"examples"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(Registry(), &s); err != nil {
		t.Fatal(err)
	}
	if s.ID != RegistryID {
		t.Errorf("$id = %q", s.ID)
	}
	c := s.Defs["Credential"]
	if c.Properties["id"].Description == "" {
		t.Error("field comments were not read")
	}
	if c.Properties["kind"].Pattern == "" || len(c.Properties["kind"].Examples) == 0 {
		t.Error("kind has no pattern or examples")
	}
	if len(c.Properties["status"].Enum) != 3 {
		t.Errorf("status enum = %v", c.Properties["status"].Enum)
	}
	for _, f := range []string{"id", "name", "system", "kind", "status", "owner", "issuer", "generation", "rotation"} {
		found := false
		for _, r := range c.Required {
			found = found || r == f
		}
		if !found {
			t.Errorf("%s is not required", f)
		}
	}
}

// TestNormalizeKeys covers the keys jsonschema builds on Windows, where
// the walked directory keeps its backslash and its "..".
func TestNormalizeKeys(t *testing.T) {
	got := normalizeKeys(map[string]string{
		ModulePath + `/schema/..\credspec.Credential`:     "windows",
		ModulePath + "/credspec.Credential.id":            "unix",
		ModulePath + `/schema/..\credspec\sub.Type.Field`: "nested",
	})
	want := map[string]string{
		ModulePath + "/credspec.Credential":     "windows",
		ModulePath + "/credspec.Credential.id":  "unix",
		ModulePath + "/credspec/sub.Type.Field": "nested",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (got %v)", k, got[k], v, got)
		}
	}
}
