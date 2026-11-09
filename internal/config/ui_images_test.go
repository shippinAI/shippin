package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestAnImageServerIsChosenByNameFromConfiguration(t *testing.T) {
	remote := UIImageRemote{Name: "mirror", Server: "https://images.test", Protocol: "simplestreams"}
	u := UI{Providers: []UIProvider{{Kind: "incus", ImageRemotes: []UIImageRemote{remote}}}}
	got, alias, ok := u.ImageRemote("incus", "mirror:debian/13")
	if !ok || got != remote || alias != "debian/13" {
		t.Fatalf("%+v %q %v", got, alias, ok)
	}
	for _, id := range []string{"elsewhere:debian/13", "local:abc", "debian/13", "mirror:"} {
		if _, _, ok := u.ImageRemote("incus", id); ok {
			t.Errorf("%q must not resolve", id)
		}
	}
	if _, _, ok := u.ImageRemote("proxmox", "mirror:debian/13"); ok {
		t.Error("a remote belongs to its provider")
	}
}

func TestAnImageServerMustBeSafeToPullFrom(t *testing.T) {
	for name, r := range map[string]UIImageRemote{
		"plain http":   {Name: "m", Server: "http://images.test", Protocol: "simplestreams"},
		"no host":      {Name: "m", Server: "https://", Protocol: "simplestreams"},
		"not a url":    {Name: "m", Server: "images.test", Protocol: "simplestreams"},
		"odd protocol": {Name: "m", Server: "https://images.test", Protocol: "ftp"},
		"takes local":  {Name: "local", Server: "https://images.test", Protocol: "simplestreams"},
		"bad name":     {Name: "a b", Server: "https://images.test", Protocol: "simplestreams"},
	} {
		if err := validImageRemote("incus", r); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if err := validImageRemote("incus", UIImageRemote{Name: "m", Server: "https://images.test", Protocol: "simplestreams"}); err != nil {
		t.Fatal(err)
	}
}

// Every image the shipped configuration offers has to name a server it also ships.
func TestShippedImagesAllHaveAServer(t *testing.T) {
	raw, err := os.ReadFile("../../config/shippin.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		UI UI `json:"ui"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, p := range cfg.UI.Providers {
		for _, i := range p.Images {
			seen++
			if _, _, ok := cfg.UI.ImageRemote(p.Kind, i.ID); !ok && !strings.HasPrefix(i.ID, "local:") {
				t.Errorf("%s offers %s with no image server", p.Kind, i.ID)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no images were checked")
	}
}
