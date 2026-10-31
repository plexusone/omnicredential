// Command gen writes the registry's and the event log's JSON Schemas into
// the schema package. Run it with go generate from the schema directory.
package main

import (
	"fmt"
	"os"

	"github.com/plexusone/omnicredential/schema"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	registry, events, err := schema.Generate("..")
	if err != nil {
		return err
	}
	if err := os.WriteFile(schema.RegistryFile, registry, 0o644); err != nil { //nolint:gosec // G306: a schema meant to be committed and read.
		return err
	}
	return os.WriteFile(schema.EventsFile, events, 0o644) //nolint:gosec // G306: a schema meant to be committed and read.
}
