package main

import "strings"

const (
	EstateControl    = "wardenclyffe-control"
	EstateAIaaS      = "aiaas"
	EstateHomelab    = "homelab"
	EstateMediastack = "mediastack"
	EstateProduction = "production"

	VisibilityInternal = "internal"
	VisibilityPublic   = "public"
)

func normalizeEstate(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case EstateControl:
		return EstateControl
	case EstateAIaaS:
		return EstateAIaaS
	case EstateMediastack:
		return EstateMediastack
	case EstateProduction:
		return EstateProduction
	case EstateHomelab:
		return EstateHomelab
	case "":
		return EstateAIaaS
	default:
		return EstateAIaaS
	}
}

func normalizeVisibility(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case VisibilityPublic:
		return VisibilityPublic
	default:
		return VisibilityInternal
	}
}

func estateForTemplate(t CatalogTemplate) string {
	if t.Estate != "" {
		return normalizeEstate(t.Estate)
	}
	if strings.EqualFold(t.Category, "media") {
		return EstateMediastack
	}
	return EstateAIaaS
}

func visibilityForTemplate(t CatalogTemplate) string {
	if t.DefaultVisibility != "" {
		return normalizeVisibility(t.DefaultVisibility)
	}
	for _, p := range t.Ports {
		if p.Public {
			return VisibilityPublic
		}
	}
	return VisibilityInternal
}
