// Command mcplspec writes what is derived from the compiled action registry: the panel's typed action
// map and the actions' OpenAPI document. It runs after cmd/mcplgen, which writes the registry. With
// -check it changes nothing and fails if they are out of date.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/contract"
)

func main() {
	check := flag.Bool("check", false, "fail if the generated files are out of date, without writing")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	files, err := contract.Files(actions.All)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcplspec:", err)
		os.Exit(1)
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	stale := false
	for _, path := range paths {
		full := filepath.Join(*root, path)
		have, _ := os.ReadFile(full)
		if bytes.Equal(have, files[path]) {
			continue
		}
		if *check {
			fmt.Fprintln(os.Stderr, "mcplspec: out of date:", path)
			stale = true
			continue
		}
		if err := os.WriteFile(full, files[path], 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "mcplspec:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
	if stale {
		os.Exit(1)
	}
}
