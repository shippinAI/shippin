package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestInstallURL(t *testing.T) {
	url, err := installURL("shippin-run")
	if err != nil || url != "https://github.com/apps/shippin-run/installations/new" {
		t.Fatalf("%s %v", url, err)
	}
	if _, err := installURL("not a slug"); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := installURL(""); err == nil {
		t.Fatal("empty slug")
	}
}

func TestExampleConfigDecodes(t *testing.T) {
	b, err := os.ReadFile("../../config/shippin-run.example.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var file fileConfig
	if err := dec.Decode(&file); err != nil {
		t.Fatal(err)
	}
	if file.FreeMinutes != 3000 || len(file.Sizes) != 4 || file.Provisioner != "none" {
		t.Fatalf("%+v", file)
	}
}
