// Command gen writes the registry's and the event log's JSON Schemas into
// the schema package. Run it with go generate from the schema directory.
package main

import (
	"fmt"
	"os"

	oscfs "github.com/grokify/oscompat/fs"

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
	// Schemas are committed and read.
	if err := oscfs.WriteFile(schema.RegistryFile, registry, oscfs.DefaultFilePerm); err != nil {
		return err
	}
	return oscfs.WriteFile(schema.EventsFile, events, oscfs.DefaultFilePerm)
}
