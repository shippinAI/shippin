package apps

import "shippin.cloud/console/internal/chat"

func registerCopyAllowlist() {
	chat.AllowCopy(IsOfficialCommand)
}
