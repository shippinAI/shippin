// Blank-import the kind packages so their init() functions register the
// connector constructors before any test in this package runs. Without
// this, tests would see host.ErrUnknownKind for every kind.
//
// This file is in the external host_test package so host itself does not
// import connector kinds (which would create an import cycle).
//
//go:build test_kinds_registered || !no_kinds

package host_test

import (
	_ "warden/internal/host/kinds/bare"
	_ "warden/internal/host/kinds/proxmox"
	_ "warden/internal/host/kinds/wardenclyffe"
)
