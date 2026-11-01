// Package schema holds the JSON Schemas of the registry and the event
// log, generated from the Go types and their documentation comments.
//
// Regenerate them after changing the types:
//
//	go generate ./schema
package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"reflect"

	oscfs "github.com/grokify/oscompat/fs"
	"github.com/invopop/jsonschema"

	"github.com/plexusone/systemspec-credential/credspec"
)

//go:generate go run ./gen

// ModulePath is the module the types belong to.
const ModulePath = "github.com/plexusone/systemspec-credential"

// Schema IDs.
const (
	RegistryID = "https://github.com/plexusone/systemspec-credential/schema/registry.schema.json"
	EventsID   = "https://github.com/plexusone/systemspec-credential/schema/events.schema.json"
)

// File names, relative to this package's directory.
const (
	RegistryFile = "registry.schema.json"
	EventsFile   = "events.schema.json"
)

//go:embed registry.schema.json
var registry []byte

//go:embed events.schema.json
var events []byte

// Registry returns the registry's JSON Schema.
func Registry() []byte { return registry }

// Events returns the event log's JSON Schema.
func Events() []byte { return events }

// Generate builds both schemas from the Go types. root is the module's
// directory, absolute or relative to the working directory: the types'
// documentation comments are read from its source.
func Generate(root string) (registry, events []byte, err error) {
	r, err := reflector(root)
	if err != nil {
		return nil, nil, err
	}
	reg := r.Reflect(&credspec.Registry{})
	reg.ID = RegistryID
	reg.Title = "Credential registry"
	reg.Description = "Every credential of a set of systems, without values: what each is for, how and where it is made, what it may do, where it is stored, who uses it and how it rotates."
	refine(reg)

	ev := r.Reflect(&credspec.Log{})
	ev.ID = EventsID
	ev.Title = "Credential events"
	ev.Description = "The append-only history of a registry's credentials: when each was created, rotated, verified, given an expiry or revoked."
	refine(ev)

	if registry, err = marshal(reg); err != nil {
		return nil, nil, err
	}
	if events, err = marshal(ev); err != nil {
		return nil, nil, err
	}
	return registry, events, nil
}

func reflector(root string) (*jsonschema.Reflector, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	cwd, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(absRoot, cwd)
	if err != nil {
		return nil, err
	}
	// jsonschema keys comments by path.Join(base, directory walked), so
	// a base that undoes the walk's relative root yields import paths.
	base := path.Join(ModulePath, oscfs.NormalizePath(rel))
	r := &jsonschema.Reflector{Mapper: enums}
	if err := r.AddGoComments(base, root); err != nil {
		return nil, fmt.Errorf("schema: reading comments: %w", err)
	}
	r.CommentMap = normalizeKeys(r.CommentMap)
	return r, nil
}

// normalizeKeys returns comments keyed by clean import paths.
// jsonschema joins the base with each directory as the platform spells it,
// so on Windows a key reads "<base>/..\credspec.Credential" and matches no
// type; with forward slashes and ".." resolved it reads
// "<module>/credspec.Credential" everywhere.
func normalizeKeys(comments map[string]string) map[string]string {
	out := make(map[string]string, len(comments))
	for k, v := range comments {
		out[oscfs.NormalizePath(k)] = v
	}
	return out
}

// enums maps the closed sets to string enums.
func enums(t reflect.Type) *jsonschema.Schema {
	var values []any
	switch t {
	case reflect.TypeFor[credspec.Status]():
		values = anys(credspec.Statuses())
	case reflect.TypeFor[credspec.Sensitivity]():
		values = anys(credspec.Sensitivities())
	case reflect.TypeFor[credspec.Method]():
		values = anys(credspec.Methods())
	case reflect.TypeFor[credspec.Role]():
		values = anys(credspec.Roles())
	case reflect.TypeFor[credspec.Policy]():
		values = anys(credspec.Policies())
	case reflect.TypeFor[credspec.EventType]():
		values = anys(credspec.EventTypes())
	case reflect.TypeFor[credspec.Actor]():
		values = anys(credspec.Actors())
	default:
		return nil
	}
	return &jsonschema.Schema{Type: "string", Enum: values}
}

func anys[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// refine adds the patterns and examples the types cannot carry.
func refine(s *jsonschema.Schema) {
	set := func(def, prop string, f func(*jsonschema.Schema)) {
		d, ok := s.Definitions[def]
		if !ok {
			return
		}
		if p, ok := d.Properties.Get(prop); ok {
			f(p)
		}
	}
	slugWith := func(examples []string) func(*jsonschema.Schema) {
		return func(p *jsonschema.Schema) {
			p.Pattern = credspec.SlugPattern
			p.Examples = anys(examples)
		}
	}
	set("Credential", "id", func(p *jsonschema.Schema) { p.Pattern = credspec.IDPattern })
	set("Credential", "kind", slugWith(credspec.Kinds()))
	set("Issuer", "type", slugWith(credspec.Issuers()))
	set("Store", "type", slugWith(credspec.StoreTypes()))
	set("Rotation", "intervalDays", func(p *jsonschema.Schema) { p.Minimum = json.Number("0") })
	for _, def := range []string{"Registry", "Log"} {
		set(def, "formatVersion", func(p *jsonschema.Schema) { p.Enum = []any{credspec.FormatVersion} })
	}
	set("Issuer", "console", func(p *jsonschema.Schema) { p.Format = "uri" })
	set("Link", "url", func(p *jsonschema.Schema) { p.Format = "uri" })
}

func marshal(s *jsonschema.Schema) ([]byte, error) {
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return append(out, '\n'), nil
}
