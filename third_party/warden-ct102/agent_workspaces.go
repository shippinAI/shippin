package main

import "net/http"

type AgentWorkspace struct {
	Name        string
	Role        string
	Endpoint    string
	WorkingPath string
	Status      string
	StatusClass string
}

type AgentLaunchGroup struct {
	Name    string
	Actions []AgentLaunchAction
}

type AgentLaunchAction struct {
	Name        string
	Runtime     string
	Target      string
	Command     string
	WorkingPath string
	Status      string
	StatusClass string
}

type AgentWorkspacesPageData struct {
	HeaderData
	PrimaryPath string
	Workspaces  []AgentWorkspace
	Groups      []AgentLaunchGroup
}

func buildAgentWorkspacesPageData(r *http.Request) AgentWorkspacesPageData {
	hd := buildHeaderData(r)
	hd.PageTitle = "Agent Workspaces"

	sharedDevPath := "/workspace/warden-storage/projects/WardenClyffe-latest"
	localSharedPath := "/mnt/warden/storage/projects/WardenClyffe-latest"

	return AgentWorkspacesPageData{
		HeaderData:  hd,
		PrimaryPath: sharedDevPath,
		Workspaces: []AgentWorkspace{
			{
				Name:        "Warden Devstation 01",
				Role:        "Daily coding workspace",
				Endpoint:    "devstation.node.ai",
				WorkingPath: sharedDevPath,
				Status:      "Mounted",
				StatusClass: "green",
			},
			{
				Name:        "Warden Operator Capsule",
				Role:        "Infra and secret-sensitive operations",
				Endpoint:    "capsule.node.ai",
				WorkingPath: "/workspace/WardenClyffe-latest",
				Status:      "Brokered SMB",
				StatusClass: "blue",
			},
			{
				Name:        "Workstation WSL",
				Role:        "Local launcher and sync client",
				Endpoint:    "wsl",
				WorkingPath: localSharedPath,
				Status:      "Mounted",
				StatusClass: "green",
			},
			{
				Name:        "Warden Shared Storage 01",
				Role:        "Project and artifact authority",
				Endpoint:    "configured-smb-share",
				WorkingPath: "/srv/warden/storage",
				Status:      "Internal only",
				StatusClass: "orange",
			},
		},
		Groups: []AgentLaunchGroup{
			{
				Name: "Local desktop app",
				Actions: []AgentLaunchAction{
					{
						Name:        "VS Code",
						Runtime:     "Remote-SSH",
						Target:      "devstation.node.ai",
						Command:     `scripts\local\open-warden-devstation-vscode.cmd`,
						WorkingPath: sharedDevPath,
						Status:      "Full extension path",
						StatusClass: "green",
					},
					{
						Name:        "Cursor",
						Runtime:     "Remote-SSH",
						Target:      "devstation.node.ai",
						Command:     `scripts\local\open-warden-devstation-cursor.cmd`,
						WorkingPath: sharedDevPath,
						Status:      "Full extension path",
						StatusClass: "green",
					},
					{
						Name:        "Browser IDE",
						Runtime:     "SSH tunnel",
						Target:      "code.devstation.node.ai",
						Command:     `scripts\local\open-warden-devstation-browser-ide.cmd`,
						WorkingPath: sharedDevPath,
						Status:      "Private fallback",
						StatusClass: "blue",
					},
				},
			},
			{
				Name: "Agents on devstation",
				Actions: []AgentLaunchAction{
					{
						Name:        "Codex CLI",
						Runtime:     "codex",
						Target:      "devstation.node.ai",
						Command:     `scripts\local\open-warden-devstation-codex.cmd`,
						WorkingPath: sharedDevPath,
						Status:      "SSH stream",
						StatusClass: "green",
					},
					{
						Name:        "Claude Code",
						Runtime:     "claude",
						Target:      "devstation.node.ai",
						Command:     `scripts\local\open-warden-devstation-claude.cmd`,
						WorkingPath: sharedDevPath,
						Status:      "SSH stream",
						StatusClass: "green",
					},
				},
			},
			{
				Name: "Infra capsule",
				Actions: []AgentLaunchAction{
					{
						Name:        "Capsule Codex",
						Runtime:     "codex",
						Target:      "capsule.node.ai",
						Command:     `scripts\local\open-warden-capsule-codex.cmd`,
						WorkingPath: "/workspace/WardenClyffe-latest",
						Status:      "SSH stream",
						StatusClass: "blue",
					},
					{
						Name:        "Capsule Claude",
						Runtime:     "claude",
						Target:      "capsule.node.ai",
						Command:     `scripts\local\open-warden-capsule-claude.cmd`,
						WorkingPath: "/workspace/WardenClyffe-latest",
						Status:      "SSH stream",
						StatusClass: "blue",
					},
				},
			},
		},
	}
}
