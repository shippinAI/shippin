package api

import (
	"context"
	"log"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// hypervisorPort is ports.Hypervisors on this panel's server registry, its sealed credentials and its
// operations log. The rules for a server or a guest are in their own folders under
// internal/infrastructure; nothing here decides anything.
type hypervisorPort struct{ a *API }

func (p hypervisorPort) Servers(ctx context.Context) ([]servers.Endpoint, error) {
	return p.a.hvDir().Endpoints(ctx)
}

func (p hypervisorPort) Server(ctx context.Context, id string) (servers.Endpoint, error) {
	return p.a.hvDir().Endpoint(ctx, id)
}

func (p hypervisorPort) Open(ep servers.Endpoint) (hv.Driver, error) { return p.a.openDriver(ep) }

func (p hypervisorPort) Track(ctx context.Context, op servers.Operation, change func(context.Context) (hv.Guest, error)) (hv.Guest, error) {
	return p.a.track(ctx, op, change)
}

func (p hypervisorPort) RecentOutcomes(ctx context.Context, server string) map[string]string {
	recent, err := p.a.hvDir().RecentOutcomes(ctx, server, inventoryLag)
	if err != nil {
		log.Printf("infra recent outcomes %s: %v", server, err)
		return nil
	}
	return recent
}

func (p hypervisorPort) ImageSource(ctx context.Context, kind, image string) (alias, server, protocol string, ok bool) {
	remote, alias, ok := p.a.uiConfig(ctx).ImageRemote(kind, image)
	return alias, remote.Server, remote.Protocol, ok
}

func (p hypervisorPort) OutsideReview(ctx context.Context) bool {
	return p.a.externalReviewConnected(ctx)
}

func (p hypervisorPort) ReviewedPower(ctx context.Context, in ports.ReviewedPower) (hv.Guest, error) {
	ep, err := p.a.hvDir().Endpoint(ctx, in.Server)
	if err != nil {
		return hv.Guest{}, err
	}
	g, problem := p.a.reviewedPower(ctx, powerCall{endpoint: ep, serverID: in.Server, guest: in.Guest, verb: in.Verb, reviewID: in.Review,
		actor: in.Actor, actorName: in.Actor, note: in.Reason, confirmSelf: in.ConfirmSelf, maySelf: in.MaySelf})
	if problem != nil {
		return hv.Guest{}, &mcpl.Error{Status: problem.status, Message: problem.msg}
	}
	return g, nil
}
