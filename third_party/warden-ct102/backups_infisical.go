package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const defaultInfisicalProjectID = "4a897376-3cbd-4aeb-8550-c7d3ed7ad113"

type infisicalS3Status struct {
	Ready      bool   `json:"ready"`
	Source     string `json:"source"`
	Message    string `json:"message"`
	HaveAccess bool   `json:"haveAccess"`
	HaveSecret bool   `json:"haveSecret"`
	HaveBucket bool   `json:"haveBucket"`
	HaveEndpt  bool   `json:"haveEndpoint"`
}

var (
	infisicalMu       sync.Mutex
	infisicalCached   infisicalS3Status
	infisicalAt       time.Time
	infisicalDisabled bool
)

func infisicalProjectID() string {
	if p := strings.TrimSpace(os.Getenv("INFISICAL_PROJECT_ID")); p != "" {
		return p
	}
	return defaultInfisicalProjectID
}

func infisicalEnvName() string {
	if p := strings.TrimSpace(os.Getenv("INFISICAL_ENV")); p != "" {
		return p
	}
	return "dev"
}

func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func s3AccessKey() string {
	return envFirst("SHIPPIN_S3_ACCESS_KEY", "SYSTEMFREAKS_S3_ACCESS_KEY", "SYSTEMFREAKS_ACCESS_KEY", "AWS_ACCESS_KEY_ID", "S3_ACCESS_KEY")
}

func s3SecretKey() string {
	return envFirst("SHIPPIN_S3_SECRET_KEY", "SYSTEMFREAKS_S3_SECRET_KEY", "SYSTEMFREAKS_SECRET_KEY", "AWS_SECRET_ACCESS_KEY", "S3_SECRET_KEY")
}

func s3EndpointEnv() string {
	return envFirst("SHIPPIN_S3_ENDPOINT", "SYSTEMFREAKS_S3_ENDPOINT", "AWS_ENDPOINT_URL_S3", "AWS_ENDPOINT_URL", "S3_ENDPOINT")
}

func s3BucketEnv() string {
	return envFirst("SHIPPIN_S3_BUCKET", "SYSTEMFREAKS_S3_BUCKET", "AWS_S3_BUCKET", "S3_BUCKET")
}

func s3RegionEnv() string {
	return envFirst("SHIPPIN_S3_REGION", "SYSTEMFREAKS_S3_REGION", "AWS_REGION", "AWS_DEFAULT_REGION", "S3_REGION")
}

func s3PrefixEnv() string {
	return envFirst("SHIPPIN_S3_PREFIX", "SYSTEMFREAKS_S3_PREFIX", "S3_PREFIX")
}

func currentInfisicalS3Status() infisicalS3Status {
	st := infisicalS3Status{
		HaveAccess: destAccessKey(estateDestShippinS3) != "" || s3AccessKey() != "",
		HaveSecret: destSecretKey(estateDestShippinS3) != "" || s3SecretKey() != "",
		HaveBucket: s3BucketEnv() != "",
		HaveEndpt:  s3EndpointEnv() != "",
	}
	st.Ready = st.HaveAccess && st.HaveSecret
	switch {
	case destHasUIKeys(estateDestShippinS3):
		st.Source = "ui"
		st.Message = "S3 keys saved in the backup panel"
	case st.Ready && os.Getenv("SHIPPIN_S3_ACCESS_KEY") != "":
		st.Source = "infisical"
		st.Message = "S3 keys loaded from Infisical"
	case st.Ready:
		st.Source = "env"
		st.Message = "S3 keys loaded from environment"
	default:
		st.Source = "missing"
		st.Message = "No S3 access/secret in Infisical (project " + infisicalProjectID() + " /shippin) or the panel yet"
	}
	return st
}

func ensureInfisicalS3() infisicalS3Status {
	infisicalMu.Lock()
	defer infisicalMu.Unlock()
	if infisicalDisabled {
		return currentInfisicalS3Status()
	}
	if time.Since(infisicalAt) < 30*time.Second && infisicalCached.Source != "" && infisicalCached.Source != "missing" {
		return infisicalCached
	}
	st := currentInfisicalS3Status()
	if _, err := exec.LookPath("infisical"); err != nil {
		if !st.Ready {
			st.Message = "Infisical CLI is not on PATH"
		}
		infisicalCached = st
		infisicalAt = time.Now()
		return st
	}
	loaded := pullInfisicalBackupSecrets()
	applyInfisicalToEstate()
	_ = ensureWardenStorageMounted()
	st = currentInfisicalS3Status()
	if loaded > 0 && st.Source == "missing" {
		st.Source = "infisical"
		st.Message = fmt.Sprintf("Infisical exported %d backup-related keys; S3 access/secret still not among them", loaded)
	} else if loaded > 0 && st.Ready {
		st.Source = "infisical"
		st.Message = "S3 keys loaded from Infisical cloud"
	}
	log.Printf("Infisical backup pull: %d keys, s3_ready=%v source=%s", loaded, st.Ready, st.Source)
	infisicalCached = st
	infisicalAt = time.Now()
	return st
}

func pullInfisicalBackupSecrets() int {
	paths := []string{"/", "/shippin", "/shippin/shared-storage", "/shippin/shared-storage/01", "/wardenclyffe", "/node"}
	if p := strings.TrimSpace(os.Getenv("INFISICAL_PATH")); p != "" {
		paths = append([]string{p}, paths...)
	}
	loaded := 0
	seen := map[string]bool{}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	for _, p := range paths {
		cmd := exec.Command("infisical", "export", "--format=dotenv", "--path="+p,
			"--projectId="+infisicalProjectID(), "--env="+infisicalEnvName(), "--silent")
		cmd.Env = append(os.Environ(), "HOME="+home)
		cmd.Dir = home
		out, err := cmd.Output()
		if err != nil {
			log.Printf("Infisical export path %s failed", p)
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			if seen[k] || !isBackupSecretName(k) {
				continue
			}
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if v == "" {
				continue
			}
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
				loaded++
			}
			seen[k] = true
		}
	}
	return loaded
}

func isBackupSecretName(k string) bool {
	u := strings.ToUpper(k)
	switch {
	case strings.HasPrefix(u, "SHIPPIN_S3_"):
		return true
	case strings.HasPrefix(u, "SYSTEMFREAKS_"):
		return true
	case strings.HasPrefix(u, "WARDEN_SHARED_STORAGE_"):
		return true
	case u == "WARDEN_STORAGE_PATH" || u == "WARDEN_OPERATOR_HOME":
		return true
	case u == "AWS_ACCESS_KEY_ID" || u == "AWS_SECRET_ACCESS_KEY" || u == "AWS_S3_BUCKET":
		return true
	case u == "AWS_ENDPOINT_URL" || u == "AWS_ENDPOINT_URL_S3" || u == "AWS_REGION" || u == "AWS_DEFAULT_REGION":
		return true
	case u == "S3_ACCESS_KEY" || u == "S3_SECRET_KEY" || u == "S3_BUCKET" || u == "S3_ENDPOINT" || u == "S3_REGION" || u == "S3_PREFIX":
		return true
	default:
		return false
	}
}

func applyInfisicalToEstate() {
	backupMu.Lock()
	seedEstateDefaultsLocked()
	for i := range backupStore.Estate.Destinations {
		if backupStore.Estate.Destinations[i].ID != estateDestShippinS3 {
			continue
		}
		if v := s3EndpointEnv(); v != "" {
			backupStore.Estate.Destinations[i].Endpoint = v
		}
		if v := s3RegionEnv(); v != "" {
			backupStore.Estate.Destinations[i].Region = v
		}
		if v := s3BucketEnv(); v != "" {
			backupStore.Estate.Destinations[i].Bucket = v
		}
		if v := s3PrefixEnv(); v != "" {
			backupStore.Estate.Destinations[i].Prefix = v
		}
	}
	for i := range backupStore.Estate.Targets {
		if backupStore.Estate.Targets[i].ID == estateTargetStorage {
			if v := strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_LINUX_MOUNT")); v != "" {
				backupStore.Estate.Targets[i].Source = v
			} else if v := strings.TrimSpace(os.Getenv("WARDEN_STORAGE_PATH")); v != "" {
				backupStore.Estate.Targets[i].Source = v
			}
		}
		if backupStore.Estate.Targets[i].ID == estateTargetHome {
			if v := strings.TrimSpace(os.Getenv("WARDEN_OPERATOR_HOME")); v != "" {
				backupStore.Estate.Targets[i].Source = v
			}
		}
	}
	backupMu.Unlock()
	_ = saveBackupStore()
	ak, sk := s3AccessKey(), s3SecretKey()
	if ak != "" || sk != "" {
		if err := mergeDestSecrets(estateDestShippinS3, ak, sk); err != nil {
			log.Printf("Infisical: failed to persist S3 keys to panel secrets file")
		}
	}
}

func ensureWardenStorageMounted() error {
	s := loadSMBShare()
	if isDirMounted(s.Mount) {
		return nil
	}
	if msg := s.missing(); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	if err := os.MkdirAll(s.Mount, 0755); err != nil {
		return err
	}
	cmd := exec.Command("mount", "-t", "cifs", s.UNC, s.Mount,
		"-o", "credentials="+s.CredFile+",uid=0,gid=0,file_mode=0644,dir_mode=0755,vers=3.0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		log.Printf("Mounted warden-storage at %s", s.Mount)
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	log.Printf("CIFS mount failed for warden-storage")
	low := strings.ToLower(msg)
	if strings.Contains(low, "operation not permitted") || strings.Contains(low, "permission denied") {
		return fmt.Errorf("this panel is an unprivileged 16G LXC and cannot kernel-mount CIFS. Warden Storage stays on LXC 117 (%s). Copies stream over SMB; kernel mount is not required", s.UNC)
	}
	return fmt.Errorf("mount: %s", msg)
}

func isDirMounted(path string) bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	want := " " + path + " "
	return strings.Contains(string(data), want)
}
