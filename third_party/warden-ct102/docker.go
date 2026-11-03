package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"` // running, exited, paused, etc.
	Ports   string `json:"ports"`
	Created string `json:"created"`
}

type DockerImage struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}

type DockerVolume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	MountPoint string `json:"mount_point"`
	Size       string `json:"size"`
}

type DockerNetwork struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

func isDockerAvailable() bool {
	_, err := os.Stat("/usr/bin/docker")
	if err != nil {
		_, err = os.Stat("/usr/local/bin/docker")
	}
	return err == nil
}

func dockerCommand(r *http.Request, args ...string) (string, error) {
	if isSudoEnabled(r) {
		_, password, _ := getSessionPassword(r)
		return sudoExecCommand(password, "docker", args...)
	}
	return execCommand("docker", args...)
}

func dockerCommandNoAuth(args ...string) (string, error) {
	return execCommand("docker", args...)
}

func getContainers(all bool) []Container {
	args := []string{"ps", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.State}}\t{{.Ports}}\t{{.CreatedAt}}", "--no-trunc"}
	if all {
		args = append(args, "-a")
	}
	out, err := dockerCommandNoAuth(args...)
	if err != nil {
		return nil
	}

	var containers []Container
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 7)
		if len(fields) < 5 {
			continue
		}
		c := Container{
			ID:    fields[0][:12],
			Name:  fields[1],
			Image: fields[2],
		}
		if len(fields) >= 4 {
			c.Status = fields[3]
		}
		if len(fields) >= 5 {
			c.State = fields[4]
		}
		if len(fields) >= 6 {
			c.Ports = fields[5]
		}
		if len(fields) >= 7 {
			c.Created = fields[6]
		}
		containers = append(containers, c)
	}
	return containers
}

func getDockerImages() []DockerImage {
	out, err := dockerCommandNoAuth("images", "--format", "{{.ID}}\t{{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}")
	if err != nil {
		return nil
	}

	var images []DockerImage
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 4 {
			continue
		}
		img := DockerImage{
			ID:         fields[0][:12],
			Repository: fields[1],
			Tag:        fields[2],
			Size:       fields[3],
		}
		if len(fields) >= 5 {
			img.Created = fields[4]
		}
		images = append(images, img)
	}
	return images
}

func getDockerVolumes() []DockerVolume {
	out, err := dockerCommandNoAuth("volume", "ls", "--format", "{{.Name}}\t{{.Driver}}\t{{.Mountpoint}}")
	if err != nil {
		return nil
	}

	var volumes []DockerVolume
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 2 {
			continue
		}
		v := DockerVolume{
			Name:   fields[0],
			Driver: fields[1],
		}
		if len(fields) >= 3 {
			v.MountPoint = fields[2]
		}
		volumes = append(volumes, v)
	}
	return volumes
}

func getDockerNetworks() []DockerNetwork {
	out, err := dockerCommandNoAuth("network", "ls", "--format", "{{.ID}}\t{{.Name}}\t{{.Driver}}\t{{.Scope}}")
	if err != nil {
		return nil
	}

	var networks []DockerNetwork
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 3 {
			continue
		}
		n := DockerNetwork{
			ID:     fields[0][:12],
			Name:   fields[1],
			Driver: fields[2],
		}
		if len(fields) >= 4 {
			n.Scope = fields[3]
		}
		networks = append(networks, n)
	}
	return networks
}

func getContainerLogs(name string, lines int) string {
	args := []string{"logs", "--tail", "100", name}
	if lines > 0 {
		args[2] = strings.Replace(args[2], "100", string(rune(lines)), 1)
	}
	out, err := dockerCommandNoAuth(args...)
	if err != nil {
		// Docker logs go to stderr for some containers
		return "Failed to retrieve logs"
	}
	if len(out) > 10000 {
		out = out[len(out)-10000:]
	}
	return out
}

func validateContainerName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '/' || c == ':') {
			return false
		}
	}
	return true
}

// --- Handlers ---

func handleDocker(w http.ResponseWriter, r *http.Request) {
	available := isDockerAvailable()

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "containers"
	}

	var containers []Container
	var images []DockerImage
	var volumes []DockerVolume
	var networks []DockerNetwork
	var containerLogs string
	logContainer := r.URL.Query().Get("logs")

	if available {
		switch tab {
		case "containers":
			containers = getContainers(true)
			if logContainer != "" && validateContainerName(logContainer) {
				containerLogs = getContainerLogs(logContainer, 100)
			}
		case "images":
			images = getDockerImages()
		case "volumes":
			volumes = getDockerVolumes()
		case "networks":
			networks = getDockerNetworks()
		}
	}

	hd := buildHeaderData(r)
	hd.PageTitle = "Docker"
	data := struct {
		HeaderData
		Available     bool
		Tab           string
		Containers    []Container
		Images        []DockerImage
		Volumes       []DockerVolume
		Networks      []DockerNetwork
		Message       string
		LogContainer  string
		ContainerLogs string
	}{
		HeaderData:    hd,
		Available:     available,
		Tab:           tab,
		Containers:    containers,
		Images:        images,
		Volumes:       volumes,
		Networks:      networks,
		Message:       r.URL.Query().Get("msg"),
		LogContainer:  logContainer,
		ContainerLogs: containerLogs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "docker.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleDockerAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.ParseForm()
	action := r.FormValue("action")
	target := r.FormValue("target")

	validActions := map[string]bool{
		"start": true, "stop": true, "restart": true, "pause": true, "unpause": true,
		"remove": true, "remove-image": true, "remove-volume": true, "pull": true,
	}
	if !validActions[action] {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if !validateContainerName(target) {
		http.Error(w, "Invalid target name", http.StatusBadRequest)
		return
	}

	var args []string
	tab := "containers"

	switch action {
	case "start":
		args = []string{"start", target}
	case "stop":
		args = []string{"stop", target}
	case "restart":
		args = []string{"restart", target}
	case "pause":
		args = []string{"pause", target}
	case "unpause":
		args = []string{"unpause", target}
	case "remove":
		args = []string{"rm", "-f", target}
	case "remove-image":
		args = []string{"rmi", target}
		tab = "images"
	case "remove-volume":
		args = []string{"volume", "rm", target}
		tab = "volumes"
	case "pull":
		args = []string{"pull", target}
		tab = "images"
	}

	var out string
	var err error
	if isSudoEnabled(r) {
		_, password, _ := getSessionPassword(r)
		out, err = sudoExecCommand(password, "docker", args...)
	} else {
		out, err = execCommand("docker", args...)
	}

	msg := action + " " + target + ": success"
	if err != nil {
		msg = action + " " + target + ": " + strings.TrimSpace(out)
		if msg == action+" "+target+": " {
			msg = action + " " + target + ": failed"
		}
	}

	if err == nil {
		auditFromRequest(r, "Docker "+action, target, "docker")
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": err == nil,
			"message": msg,
		})
		return
	}

	http.Redirect(w, r, "/docker?tab="+tab+"&msg="+msg, http.StatusSeeOther)
}
