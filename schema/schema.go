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

	"github.com/invopop/jsonschema"

	"github.com/plexusone/omnicredential"
)

//go:generate go run ./gen

// ModulePath is the module the types belong to.
const ModulePath = "github.com/plexusone/omnicredential"

// Schema IDs.
const (
	RegistryID = "https://github.com/plexusone/omnicredential/schema/registry.schema.json"
	EventsID   = "https://github.com/plexusone/omnicredential/schema/events.schema.json"
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
	reg := r.Reflect(&omnicredential.Registry{})
	reg.ID = RegistryID
	reg.Title = "Credential registry"
	reg.Description = "Every credential of a set of systems, without values: what each is for, how and where it is made, what it may do, where it is stored, who uses it and how it rotates."
	refine(reg)

	ev := r.Reflect(&omnicredential.Log{})
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
	base := path.Join(ModulePath, filepath.ToSlash(rel))
	r := &jsonschema.Reflector{Mapper: enums}
	if err := r.AddGoComments(base, root); err != nil {
		return nil, fmt.Errorf("schema: reading comments: %w", err)
	}
	return r, nil
}

// enums maps the closed sets to string enums.
func enums(t reflect.Type) *jsonschema.Schema {
	var values []any
	switch t {
	case reflect.TypeFor[omnicredential.Status]():
		values = anys(omnicredential.Statuses())
	case reflect.TypeFor[omnicredential.Sensitivity]():
		values = anys(omnicredential.Sensitivities())
	case reflect.TypeFor[omnicredential.Method]():
		values = anys(omnicredential.Methods())
	case reflect.TypeFor[omnicredential.Role]():
		values = anys(omnicredential.Roles())
	case reflect.TypeFor[omnicredential.Policy]():
		values = anys(omnicredential.Policies())
	case reflect.TypeFor[omnicredential.EventType]():
		values = anys(omnicredential.EventTypes())
	case reflect.TypeFor[omnicredential.Actor]():
		values = anys(omnicredential.Actors())
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
			p.Pattern = omnicredential.SlugPattern
			p.Examples = anys(examples)
		}
	}
	set("Credential", "id", func(p *jsonschema.Schema) { p.Pattern = omnicredential.IDPattern })
	set("Credential", "kind", slugWith(omnicredential.Kinds()))
	set("Issuer", "type", slugWith(omnicredential.Issuers()))
	set("Store", "type", slugWith(omnicredential.StoreTypes()))
	set("Rotation", "intervalDays", func(p *jsonschema.Schema) { p.Minimum = json.Number("0") })
	for _, def := range []string{"Registry", "Log"} {
		set(def, "formatVersion", func(p *jsonschema.Schema) { p.Enum = []any{omnicredential.FormatVersion} })
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
