package hv

import (
	"slices"
	"testing"
)

func TestStorageInventoryCapabilityIsProviderDeclared(t *testing.T) {
	proxmox, ok := SpecFor("proxmox")
	if !ok || !slices.Contains(proxmox.Capabilities, CapStorageInventory) {
		t.Fatalf("Proxmox storage inventory capability missing: %+v", proxmox)
	}
	incus, ok := SpecFor("incus")
	if !ok || slices.Contains(incus.Capabilities, CapStorageInventory) {
		t.Fatalf("Incus must not advertise the Proxmox-only storage inventory route: %+v", incus)
	}
}
