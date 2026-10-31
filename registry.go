package omnicredential

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Registry is a list of credentials.
type Registry struct {
	// Schema is the JSON Schema the file names, if any.
	Schema string `json:"$schema,omitempty"`
	// FormatVersion is FormatVersion.
	FormatVersion string `json:"formatVersion"`
	// Credentials are the entries, in the order given.
	Credentials []Credential `json:"credentials"`
}

// NewRegistry returns a registry of the credentials, in the current
// format.
func NewRegistry(creds ...Credential) *Registry {
	return &Registry{FormatVersion: FormatVersion, Credentials: creds}
}

// Add appends credentials.
func (r *Registry) Add(creds ...Credential) { r.Credentials = append(r.Credentials, creds...) }

// Find returns the credential with the ID.
func (r *Registry) Find(id string) (Credential, bool) {
	for _, c := range r.Credentials {
		if c.ID == id {
			return c, true
		}
	}
	return Credential{}, false
}

var (
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`)
	slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

// IDPattern is the pattern a credential ID matches.
const IDPattern = `^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`

// SlugPattern is the pattern of open values such as kinds, issuer types
// and store types.
const SlugPattern = `^[a-z0-9][a-z0-9-]*$`

// Problem is one thing wrong with a registry or an event log.
type Problem struct {
	// ID is the credential's ID, or the event's position, when known.
	ID string
	// Message says what is wrong.
	Message string
}

func (p Problem) String() string {
	if p.ID == "" {
		return p.Message
	}
	return p.ID + ": " + p.Message
}

// ValidationError lists every problem found.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.String()
	}
	return fmt.Sprintf("%d problem(s):\n  %s", len(e.Problems), strings.Join(lines, "\n  "))
}

type problems []Problem

func (ps *problems) add(id, format string, args ...any) {
	*ps = append(*ps, Problem{ID: id, Message: fmt.Sprintf(format, args...)})
}

func (ps problems) err() error {
	if len(ps) == 0 {
		return nil
	}
	return &ValidationError{Problems: ps}
}

// Validate checks the registry and returns a *ValidationError listing
// every problem, or nil.
func (r *Registry) Validate() error {
	var ps problems
	if r.FormatVersion != FormatVersion {
		ps.add("", "formatVersion is %q, want %q", r.FormatVersion, FormatVersion)
	}
	ids := make(map[string]bool, len(r.Credentials))
	for i, c := range r.Credentials {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("credentials[%d]", i)
			ps.add(id, "id is required")
		} else if !idPattern.MatchString(c.ID) {
			ps.add(id, "id must match %s", IDPattern)
		}
		if c.ID != "" {
			if ids[c.ID] {
				ps.add(id, "duplicate id")
			}
			ids[c.ID] = true
		}
	}
	for i, c := range r.Credentials {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("credentials[%d]", i)
		}
		validateCredential(&ps, id, c, ids)
	}
	return ps.err()
}

func validateCredential(ps *problems, id string, c Credential, ids map[string]bool) {
	required := []struct{ name, value string }{
		{"name", c.Name}, {"system", c.System}, {"owner", c.Owner},
	}
	for _, f := range required {
		if strings.TrimSpace(f.value) == "" {
			ps.add(id, "%s is required", f.name)
		}
	}
	slug(ps, id, "kind", c.Kind, true)
	closed(ps, id, "status", c.Status, Statuses(), true)
	closed(ps, id, "sensitivity", c.Sensitivity, Sensitivities(), false)
	slug(ps, id, "issuer.type", c.Issuer.Type, true)
	httpsURL(ps, id, "issuer.console", c.Issuer.Console)
	closed(ps, id, "generation.method", c.Generation.Method, Methods(), true)

	for i, e := range c.Entitlements {
		if strings.TrimSpace(e.Resource) == "" || strings.TrimSpace(e.Access) == "" {
			ps.add(id, "entitlements[%d] needs a resource and an access", i)
		}
	}
	primary := 0
	for i, s := range c.Stores {
		slug(ps, id, fmt.Sprintf("stores[%d].type", i), s.Type, true)
		closed(ps, id, fmt.Sprintf("stores[%d].role", i), s.Role, Roles(), true)
		if s.Role == RolePrimary {
			primary++
		}
	}
	if c.Generation.Method != MethodFederated && primary == 0 {
		ps.add(id, "needs a store with role primary")
	}
	for i, u := range c.Consumers {
		if strings.TrimSpace(u.System) == "" {
			ps.add(id, "consumers[%d].system is required", i)
		}
	}

	closed(ps, id, "rotation.policy", c.Rotation.Policy, Policies(), true)
	switch {
	case c.Rotation.IntervalDays < 0:
		ps.add(id, "rotation.intervalDays must not be negative")
	case c.Rotation.Policy == PolicyScheduled && c.Rotation.IntervalDays == 0:
		ps.add(id, "rotation.intervalDays is required for the scheduled policy")
	case c.Rotation.Policy != PolicyScheduled && c.Rotation.IntervalDays != 0:
		ps.add(id, "rotation.intervalDays is only for the scheduled policy")
	}

	for _, d := range c.DependsOn {
		switch {
		case d == c.ID:
			ps.add(id, "dependsOn names itself")
		case !ids[d]:
			ps.add(id, "dependsOn names unknown credential %q", d)
		}
	}
	if c.Generation.Method == MethodDerived && len(c.DependsOn) == 0 {
		ps.add(id, "a derived credential lists what it depends on")
	}
	for i, t := range c.Tags {
		if strings.TrimSpace(t) == "" {
			ps.add(id, "tags[%d] is empty", i)
		}
	}
	for i, l := range c.Links {
		if strings.TrimSpace(l.Title) == "" {
			ps.add(id, "links[%d].title is required", i)
		}
		if l.URL == "" {
			ps.add(id, "links[%d].url is required", i)
		}
		httpsURL(ps, id, fmt.Sprintf("links[%d].url", i), l.URL)
	}
}

func slug(ps *problems, id, field, v string, required bool) {
	switch {
	case v == "" && required:
		ps.add(id, "%s is required", field)
	case v != "" && !slugPattern.MatchString(v):
		ps.add(id, "%s %q must be a lowercase slug", field, v)
	}
}

func closed[T ~string](ps *problems, id, field string, v T, allowed []T, required bool) {
	switch {
	case v == "" && required:
		ps.add(id, "%s is required", field)
	case v != "" && !slices.Contains(allowed, v):
		ps.add(id, "%s %q is not one of %v", field, v, allowed)
	}
}

func httpsURL(ps *problems, id, field, v string) {
	if v == "" {
		return
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		ps.add(id, "%s must be an absolute https URL", field)
	}
}

// ReadRegistry decodes a registry, refusing unknown fields, and
// validates it.
func ReadRegistry(data []byte) (*Registry, error) {
	var r Registry
	if err := decodeStrict(data, &r); err != nil {
		return nil, fmt.Errorf("registry: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// LoadRegistry reads and validates the registry file at path.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the caller names the registry file.
	if err != nil {
		return nil, err
	}
	r, err := ReadRegistry(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// encode writes v as JSON with a two-space indent, a final newline and
// no HTML escaping, so the file reads as written.
func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Encode returns the registry as indented JSON.
func (r *Registry) Encode() ([]byte, error) { return encode(r) }

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data after the JSON value")
	}
	return nil
}

// writeFile writes data to path through a temporary file in the same
// directory, so a reader never sees a partial file.
func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".omnicredential-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(err, tmp.Close(), os.Remove(name))
	}
	if err := tmp.Close(); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	if err := os.Chmod(name, 0o644); err != nil { //nolint:gosec // G302: the file holds no values and is meant to be read and committed.
		return errors.Join(err, os.Remove(name))
	}
	if err := os.Rename(name, path); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	return nil
}

// Save writes the registry to path as indented JSON.
func (r *Registry) Save(path string) error {
	data, err := r.Encode()
	if err != nil {
		return err
	}
	return writeFile(path, data)
}
