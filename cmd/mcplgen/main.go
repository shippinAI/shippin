// Command mcplgen writes the MCPL action registry and the panel's action map from the action files
// under internal/<domain>/. With -check it changes nothing and fails if they are out of date.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"shippin.cloud/console/internal/mcpl/gen"
)

func main() {
	check := flag.Bool("check", false, "fail if the generated files are out of date, without writing")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	files, err := gen.Files(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcplgen:", err)
		os.Exit(1)
	}
	stale := false
	for path, want := range files {
		full := filepath.Join(*root, path)
		have, _ := os.ReadFile(full)
		if bytes.Equal(have, want) {
			continue
		}
		if *check {
			fmt.Fprintln(os.Stderr, "mcplgen: out of date:", path)
			stale = true
			continue
		}
		if err := os.WriteFile(full, want, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "mcplgen:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
	if stale {
		os.Exit(1)
	}
}
