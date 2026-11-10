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

func TestSavingAnOverrideKeepsImagesItDoesNotMention(t *testing.T) {
	remote := UIImageRemote{Name: "mirror", Server: "https://images.test", Protocol: "simplestreams"}
	image := UIImage{ID: "mirror:debian/13", Label: "Debian 13"}
	file := UI{Providers: []UIProvider{{Kind: "incus", Label: "From file", Images: []UIImage{image}, ImageRemotes: []UIImageRemote{remote}}}}

	got, _ := file.Merge(UI{Providers: []UIProvider{{Kind: "incus", Label: "Renamed"}}}).Provider("incus")
	if got.Label != "Renamed" || len(got.Images) != 1 || len(got.ImageRemotes) != 1 {
		t.Fatalf("an override that only renames lost the images: %+v", got)
	}
	got, _ = file.Merge(UI{Providers: []UIProvider{{Kind: "incus", Images: []UIImage{}, ImageRemotes: []UIImageRemote{}}}}).Provider("incus")
	if got.Images == nil || len(got.Images) != 0 || got.ImageRemotes == nil || len(got.ImageRemotes) != 0 {
		t.Fatalf("an empty list must remove them: %+v", got)
	}
	var round UI
	raw, _ := json.Marshal(file.Merge(UI{Providers: []UIProvider{{Kind: "incus", Images: []UIImage{}}}}))
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if p, _ := round.Provider("incus"); p.Images == nil {
		t.Fatalf("a cleared list must survive being sent to the panel and back: %s", raw)
	}
}

func TestImagesAreBoundedAndUnique(t *testing.T) {
	remote := UIImageRemote{Name: "m", Server: "https://images.test", Protocol: "simplestreams"}
	ok := UIImage{ID: "m:debian/13", Label: "Debian 13", Kind: "ct"}
	if err := (UI{Providers: []UIProvider{{Kind: "incus", Images: []UIImage{ok}, ImageRemotes: []UIImageRemote{remote}}}}).Check(); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]UIProvider{
		"image without id":    {Kind: "incus", Images: []UIImage{{Label: "x"}}},
		"image without label": {Kind: "incus", Images: []UIImage{{ID: "x"}}},
		"unknown kind":        {Kind: "incus", Images: []UIImage{{ID: "x", Label: "x", Kind: "pod"}}},
		"duplicate image":     {Kind: "incus", Images: []UIImage{ok, ok}},
		"duplicate server":    {Kind: "incus", ImageRemotes: []UIImageRemote{remote, remote}},
		"control character":   {Kind: "incus", Images: []UIImage{{ID: "a\nb", Label: "x"}}},
		"oversized id":        {Kind: "incus", Images: []UIImage{{ID: strings.Repeat("a", 201), Label: "x"}}},
	} {
		if err := (UI{Providers: []UIProvider{p}}).Check(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
