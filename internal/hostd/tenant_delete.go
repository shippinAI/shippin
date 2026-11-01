package hostd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// DeleteTenant tears down a tenant silo: every workspace (route, ports, instance, home volume),
// then the same network rollback used by Provision, then the project and storage pool.
// The encrypted ZFS dataset is kept (unmounted data stays recoverable); it is reported back.
func (s *Server) DeleteTenant(ctx context.Context, t string) (string, error) {
	pr := "--project=" + Project(t)
	if !exists(ctx, "project", "show", Project(t)) {
		return Dataset(s.cfg.DatasetRoot, t), nil
	}
	var insts []string
	if err := query(ctx, fmt.Sprintf("/1.0/instances?project=%s", Project(t)), &insts); err != nil {
		return "", err
	}
	for _, p := range insts {
		if lastSegment(p) == TenantVM(t) {
			continue // removed below, after its nested workspaces
		}
		w := strings.TrimPrefix(lastSegment(p), "w-")
		if ValidUUID(w) != nil {
			return "", fmt.Errorf("unexpected instance %q in tenant project", p)
		}
		if err := s.removeLocked(ctx, place{t: t}, w); err != nil {
			return "", err
		}
	}
	if err := s.deleteTenantVM(ctx, t); err != nil {
		return "", err
	}
	if err := s.dropVolumes(ctx, t); err != nil {
		return "", err
	}
	for _, d := range []string{"eth0", "root"} {
		_, _ = incus(ctx, "profile", "device", "remove", "default", d, pr)
	}
	if err := s.dropNetwork(ctx, t); err != nil {
		return "", err
	}
	if exists(ctx, "network", "acl", "show", aclName, pr) {
		if _, err := incus(ctx, "network", "acl", "delete", aclName, pr); err != nil {
			return "", err
		}
	}
	if _, err := incus(ctx, "project", "delete", Project(t)); err != nil {
		return "", err
	}
	if exists(ctx, "storage", "show", Pool(t)) {
		if _, err := incus(ctx, "storage", "delete", Pool(t)); err != nil {
			return "", err
		}
	}
	return Dataset(s.cfg.DatasetRoot, t), nil
}

// dropVolumes deletes what's left in the tenant pool so it can be removed: custom h-<uuid> volumes
// (e.g. of a workspace whose launch failed) and cached image volumes (the images themselves stay
// in the host image store).
func (s *Server) dropVolumes(ctx context.Context, t string) error {
	var imgs []string
	_ = query(ctx, fmt.Sprintf("/1.0/storage-pools/%s/volumes/image", Pool(t)), &imgs)
	for _, v := range imgs {
		fp := lastSegment(v)
		if !fingerprintRe.MatchString(fp) {
			return fmt.Errorf("unexpected image volume %q in tenant pool", fp)
		}
		if _, err := incus(ctx, "storage", "volume", "delete", Pool(t), "image/"+fp); err != nil {
			return err
		}
	}
	var vols []string
	if err := query(ctx, fmt.Sprintf("/1.0/storage-pools/%s/volumes/custom?project=%s", Pool(t), Project(t)), &vols); err != nil {
		return nil // pool already gone
	}
	for _, v := range vols {
		name := lastSegment(v)
		if name != TenantVMData(t) && ValidUUID(strings.TrimPrefix(name, "h-")) != nil {
			return fmt.Errorf("unexpected volume %q in tenant pool", name)
		}
		if _, err := incus(ctx, "storage", "volume", "delete", Pool(t), name, "--project="+Project(t)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) deleteTenant(w http.ResponseWriter, r *http.Request) {
	ds, err := s.DeleteTenant(r.Context(), r.PathValue("t"))
	if err != nil {
		log.Printf("delete tenant %s: %v", r.PathValue("t"), err)
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("deleted tenant %s (dataset %s kept)", r.PathValue("t"), ds)
	reply(w, map[string]string{"dataset_kept": ds})
}

var fingerprintRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// lastSegment returns the final path segment of an Incus API URL, without its ?query
// (e.g. "/1.0/instances/w-x?project=t-y" -> "w-x").
func lastSegment(u string) string {
	u, _, _ = strings.Cut(u, "?")
	return u[strings.LastIndex(u, "/")+1:]
}
