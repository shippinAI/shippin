package apps

import "shippin.cloud/console/internal/runtime/chat"

func registerCopyAllowlist() {
	chat.AllowCopy(IsOfficialCommand)
}
