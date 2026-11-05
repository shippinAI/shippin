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
	if !ok || !slices.Contains(incus.Capabilities, CapStorageInventory) {
		t.Fatalf("Incus storage inventory capability missing: %+v", incus)
	}
}

func TestTenantSecretExposureAcknowledgementIsProviderDeclared(t *testing.T) {
	incus, ok := SpecFor("incus")
	if !ok || !slices.Contains(incus.Capabilities, CapTenantSecretExposurePolicy) {
		t.Fatalf("Incus secret exposure policy capability missing: %+v", incus)
	}
	proxmox, ok := SpecFor("proxmox")
	if !ok || slices.Contains(proxmox.Capabilities, CapTenantSecretExposurePolicy) {
		t.Fatalf("unsupported provider must not expose secret policy capability: %+v", proxmox)
	}
}
