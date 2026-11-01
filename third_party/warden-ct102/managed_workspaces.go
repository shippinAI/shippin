package main

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ManagedWorkspaceMap struct {
	Available        bool                         `json:"available" yaml:"-"`
	Path             string                       `json:"path" yaml:"-"`
	Error            string                       `json:"error,omitempty" yaml:"-"`
	Version          any                          `json:"version,omitempty" yaml:"version"`
	Status           string                       `json:"status" yaml:"status"`
	LastUpdated      string                       `json:"last_updated" yaml:"last_updated"`
	Workspaces       []ManagedWorkspace           `json:"workspaces" yaml:"workspaces"`
	Projects         []ManagedProject             `json:"projects" yaml:"projects"`
	ServiceInstances []ManagedServiceInstance     `json:"service_instances" yaml:"service_instances"`
	Summary          ManagedWorkspaceSummary      `json:"summary" yaml:"-"`
	Connections      []ManagedWorkspaceConnection `json:"connections" yaml:"-"`
}

type ManagedWorkspace struct {
	WorkspaceUUID       string            `json:"workspace_uuid" yaml:"workspace_uuid"`
	WorkspaceSlug       string            `json:"workspace_slug" yaml:"workspace_slug"`
	WorkspaceName       string            `json:"workspace_name" yaml:"workspace_name"`
	WorkspaceKind       string            `json:"workspace_kind" yaml:"workspace_kind"`
	OwnerAccountSlug    string            `json:"owner_account_slug" yaml:"owner_account_slug"`
	OwnerPersonSlug     string            `json:"owner_person_slug" yaml:"owner_person_slug"`
	Estate              string            `json:"estate" yaml:"estate"`
	Status              string            `json:"status" yaml:"status"`
	MCPRoot             string            `json:"mcp_root" yaml:"mcp_root"`
	PolicyRoot          string            `json:"policy_root" yaml:"policy_root"`
	BaselineFor         []string          `json:"baseline_for" yaml:"baseline_for"`
	ContributesToPlanes []ManagedPlaneRef `json:"contributes_to_planes" yaml:"contributes_to_planes"`
	ProjectCount        int               `json:"project_count" yaml:"-"`
	ConnectionCount     int               `json:"connection_count" yaml:"-"`
	Role                string            `json:"role" yaml:"-"`
}

type ManagedPlaneRef struct {
	Plane     string `json:"plane" yaml:"plane"`
	SurrealNS string `json:"surreal_ns" yaml:"surreal_ns"`
	SurrealDB string `json:"surreal_db" yaml:"surreal_db"`
}

type ManagedProject struct {
	ProjectUUID      string `json:"project_uuid" yaml:"project_uuid"`
	ProjectKey       string `json:"project_key" yaml:"project_key"`
	WorkspaceSlug    string `json:"workspace_slug" yaml:"workspace_slug"`
	RepoDir          string `json:"repo_dir" yaml:"repo_dir"`
	CanonicalRepoURL string `json:"canonical_repo_url" yaml:"canonical_repo_url"`
	DefaultBranch    string `json:"default_branch" yaml:"default_branch"`
	StorageID        string `json:"storage_id" yaml:"storage_id"`
	Status           string `json:"status" yaml:"status"`
}

type ManagedServiceInstance struct {
	ServiceInstanceUUID    string             `json:"service_instance_uuid" yaml:"service_instance_uuid"`
	ServiceSlug            string             `json:"service_slug" yaml:"service_slug"`
	DisplayName            string             `json:"display_name" yaml:"display_name"`
	ServiceKind            string             `json:"service_kind" yaml:"service_kind"`
	OwnerAccountSlug       string             `json:"owner_account_slug" yaml:"owner_account_slug"`
	EnvironmentSlug        string             `json:"environment_slug" yaml:"environment_slug"`
	PrimaryWorkspaceSlug   string             `json:"primary_workspace_slug" yaml:"primary_workspace_slug"`
	AttachedWorkspaceSlugs []string           `json:"attached_workspace_slugs" yaml:"attached_workspace_slugs"`
	DefaultProjectRef      ManagedProjectRef  `json:"default_project_ref" yaml:"default_project_ref"`
	Host                   ManagedServiceHost `json:"host" yaml:"host"`
}

type ManagedProjectRef struct {
	WorkspaceSlug string `json:"workspace_slug" yaml:"workspace_slug"`
	ProjectKey    string `json:"project_key" yaml:"project_key"`
}

type ManagedServiceHost struct {
	HostID       string `json:"host_id" yaml:"host_id"`
	HostKind     string `json:"host_kind" yaml:"host_kind"`
	VMID         int    `json:"vmid" yaml:"vmid"`
	IP           string `json:"ip" yaml:"ip"`
	SSHAlias     string `json:"ssh_alias" yaml:"ssh_alias"`
	OperatorUser string `json:"operator_user" yaml:"operator_user"`
	AdminUser    string `json:"admin_user" yaml:"admin_user"`
}

type ManagedWorkspaceSummary struct {
	WorkspaceCount  int `json:"workspace_count"`
	ProjectCount    int `json:"project_count"`
	ServiceCount    int `json:"service_count"`
	ConnectionCount int `json:"connection_count"`
	PlaneCount      int `json:"plane_count"`
}

type ManagedWorkspaceConnection struct {
	Kind   string `json:"kind"`
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func getManagedWorkspaceMap() ManagedWorkspaceMap {
	path, checked := resolveManagedWorkspaceRegistryPath()
	if path == "" {
		return ManagedWorkspaceMap{
			Available: false,
			Error:     "managed workspace registry not found; checked " + strings.Join(checked, ", "),
		}
	}
	return loadManagedWorkspaceMapFromPath(path)
}

func loadManagedWorkspaceMapFromPath(path string) ManagedWorkspaceMap {
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagedWorkspaceMap{Available: false, Path: path, Error: err.Error()}
	}
	var view ManagedWorkspaceMap
	if err := yaml.Unmarshal(data, &view); err != nil {
		return ManagedWorkspaceMap{Available: false, Path: path, Error: err.Error()}
	}
	view.Available = true
	view.Path = path
	view.normalize()
	return view
}

func resolveManagedWorkspaceRegistryPath() (string, []string) {
	var candidates []string
	if envPath := strings.TrimSpace(os.Getenv("WARDEN_MANAGED_WORKSPACES_REGISTRY")); envPath != "" {
		candidates = append(candidates, envPath)
	}
	candidates = append(candidates,
		filepath.Join("registry", "managed-workspaces", "workspaces.seed.yaml"),
		filepath.Join("..", "registry", "managed-workspaces", "workspaces.seed.yaml"),
		filepath.Join("/etc", "warden", "managed-workspaces", "workspaces.seed.yaml"),
	)
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate, candidates
			}
			return abs, candidates
		}
	}
	return "", candidates
}
