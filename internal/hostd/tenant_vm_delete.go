package hostd

import (
	"context"
)

func (s *Server) removeLocked(ctx context.Context, at place, w string) error {
	defer lockWorkspace(w)()
	return s.remove(ctx, at, w)
}

// deleteTenantVM removes every nested workspace (edge route, previews, ports, instance, home) through
// the VM, then the VM and its data volume. A stopped VM is started first so its workspaces can be
// listed; the edge is cleaned for each one even if the inner delete fails.
func (s *Server) deleteTenantVM(ctx context.Context, t string) error {
	pr, vm := "--project="+Project(t), TenantVM(t)
	if exists(ctx, "info", vm, pr) {
		at, err := s.newPlace(ctx, t)
		if err != nil {
			return err
		}
		if _, err := incus(ctx, "start", vm, pr); err != nil && !isRunning(err) {
			return err
		}
		if err := waitAgent(ctx, t); err != nil {
			return err
		}
		rows, err := listInner(ctx, at, 1)
		if err != nil {
			return err
		}
		items, err := inventory(rows)
		if err != nil {
			return err
		}
		for _, it := range items {
			where, err := at.atProject(it.Project)
			if err != nil {
				return err
			}
			switch it.Kind {
			case "workspace":
				if err := s.removeLocked(ctx, where, it.ID); err != nil {
					_ = s.UnpublishRoute(ctx, it.ID)
					_ = s.dropPreviews(ctx, it.ID)
					_ = s.SetPorts(ctx, it.ID, nil)
				}
			case "utility":
				_ = s.removeUtility(ctx, where, it.ID)
			}
		}
		if _, err := incus(ctx, "delete", vm, pr, "--force"); err != nil {
			return err
		}
	}
	if exists(ctx, "storage", "volume", "show", Pool(t), TenantVMData(t), pr) {
		if _, err := incus(ctx, "storage", "volume", "delete", Pool(t), TenantVMData(t), pr); err != nil {
			return err
		}
	}
	return nil
}

// removeUtility drops the activation container's edge route and the container. The AI home
// volume stays; it lives on the inner pool and goes away with the VM's data disk.
func (s *Server) removeUtility(ctx context.Context, at place, account string) error {
	_, _ = s.caddy(ctx, "DELETE", "/id/u-"+account, nil)
	inst := UtilityInstance(account)
	if !at.has(ctx, "info", inst) {
		return nil
	}
	_, err := at.do(ctx, "delete", inst, "--force")
	return err
}
