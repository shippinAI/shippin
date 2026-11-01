package main

// Customers drill-down HTTP handlers per ADR 0038 + Spec 17 §4.
// Companion to warden/customers.go. Composes components.* partials —
// no inline-style HTML in this file's output.

import (
	"log"
	"net/http"
	"strings"
)

// BreadcrumbCrumb mirrors Spec 17 §3.2.
type BreadcrumbCrumb struct {
	Label string
	Href  string
	Last  bool
}

// ShellArgs is the components.shell arg wrapper. Carries the existing
// HeaderData fields the header.html partial already needs.
type ShellArgs struct {
	HeaderData
	PageTitle    string
	Breadcrumb   []BreadcrumbCrumb
	BodyClass    string
	BodyTemplate string
}

// CustomersPageData is the customers/index page payload.
type CustomersPageData struct {
	Shell     ShellArgs
	Customers []Customer
	LoadError string
}

// CustomerDetailPageData is the /customers/<slug> overview payload.
type CustomerDetailPageData struct {
	Shell    ShellArgs
	Customer Customer
	File     *CustomerServicesFile
	Sections []CustomerOverviewSection
}

// CustomerOverviewSection is one card on the customer detail page.
type CustomerOverviewSection struct {
	Title       string
	Icon        string
	Subtitle    string
	DrillHref   string
	StatusState string
	StatusLabel string
}

// CustomerDevstationPageData is /customers/<slug>/devstation payload.
type CustomerDevstationPageData struct {
	Shell    ShellArgs
	Customer Customer
	Services []CustomerServiceInstance
}

// CustomerDevstationServicePageData is /customers/<slug>/devstation/<svc>.
type CustomerDevstationServicePageData struct {
	Shell    ShellArgs
	Customer Customer
	Service  CustomerServiceInstance
}

// handleCustomersIndex renders the customers list at /customers.
// Per ADR 0038 §4 this is a flat route (the drill happens via card href).
func handleCustomersIndex(w http.ResponseWriter, r *http.Request) {
	list := listCustomers()
	data := CustomersPageData{
		Shell: ShellArgs{
			HeaderData:   buildHeaderData(r),
			PageTitle:    "Customers",
			BodyTemplate: "customers.body",
			Breadcrumb:   []BreadcrumbCrumb{{Label: "Customers", Href: "/customers", Last: true}},
		},
		Customers: list,
		LoadError: customers.loadError,
	}
	renderTemplate(w, "customers.html", data)
}

// handleCustomerDrill routes /customers/{slug}/[...] to the right sub-handler.
// Per Spec 17 §4 dispatch table.
func handleCustomerDrill(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/customers/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.Redirect(w, r, "/customers", http.StatusFound)
		return
	}
	parts := strings.Split(rest, "/")
	slug := parts[0]
	_, ok := getCustomer(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case len(parts) == 1:
		renderCustomerDetail(w, r, slug)
	case len(parts) >= 2 && parts[1] == "devstation":
		if len(parts) == 2 {
			renderCustomerDevstation(w, r, slug)
			return
		}
		renderCustomerDevstationService(w, r, slug, parts[2])
	case len(parts) >= 2 && parts[1] == "bundles":
		renderCustomerBundles(w, r, slug)
	case len(parts) >= 2 && parts[1] == "identity":
		renderCustomerIdentity(w, r, slug)
	case len(parts) >= 2 && parts[1] == "secrets":
		renderCustomerSecrets(w, r, slug)
	case len(parts) >= 2 && parts[1] == "observability":
		renderCustomerObservability(w, r, slug)
	case len(parts) >= 2 && parts[1] == "network":
		renderCustomerNetwork(w, r, slug)
	default:
		http.NotFound(w, r)
	}
}

// renderCustomerDetail handles /customers/<slug>.
func renderCustomerDetail(w http.ResponseWriter, r *http.Request, slug string) {
	f, _ := getCustomer(slug)
	c := Customer{
		AccountSlug:  slug,
		DisplayName:  strings.Title(slug),
		BrandFront:   primaryBrandFront(f),
		Tier:         "T1",
		ServiceCount: len(f.Services),
	}

	sections := []CustomerOverviewSection{
		{Title: "Devstation", Icon: "🖥️", Subtitle: "Devstation services + runtimes", DrillHref: "/customers/" + slug + "/devstation", StatusState: "active", StatusLabel: "active"},
		{Title: "Bundles", Icon: "📦", Subtitle: "Customer bundles owned by this account", DrillHref: "/customers/" + slug + "/bundles", StatusState: "active", StatusLabel: "active"},
		{Title: "Identity", Icon: "🔐", Subtitle: "Authentik posture, passkeys, recovery", DrillHref: "/customers/" + slug + "/identity", StatusState: "active", StatusLabel: "active"},
		{Title: "Secrets", Icon: "🗝️", Subtitle: "Infisical scope, rotation log", DrillHref: "/customers/" + slug + "/secrets", StatusState: "active", StatusLabel: "active"},
		{Title: "Observability", Icon: "📈", Subtitle: "Observatory slice + Bifrost spend", DrillHref: "/customers/" + slug + "/observability", StatusState: "active", StatusLabel: "active"},
		{Title: "Network", Icon: "🌐", Subtitle: "VLAN + WardenNet handshake", DrillHref: "/customers/" + slug + "/network", StatusState: "active", StatusLabel: "active"},
	}

	data := CustomerDetailPageData{
		Shell: ShellArgs{
			HeaderData:   buildHeaderData(r),
			PageTitle:    c.DisplayName,
			BodyTemplate: "customer_detail.body",
			Breadcrumb: []BreadcrumbCrumb{
				{Label: "Customers", Href: "/customers"},
				{Label: c.DisplayName, Href: "/customers/" + slug, Last: true},
			},
		},
		Customer: c,
		File:     f,
		Sections: sections,
	}
	renderTemplate(w, "customer_detail.html", data)
}

// renderCustomerDevstation handles /customers/<slug>/devstation.
func renderCustomerDevstation(w http.ResponseWriter, r *http.Request, slug string) {
	c := Customer{AccountSlug: slug, DisplayName: strings.Title(slug), Tier: "T1"}
	data := CustomerDevstationPageData{
		Shell: ShellArgs{
			HeaderData:   buildHeaderData(r),
			PageTitle:    c.DisplayName + " — Devstation",
			BodyTemplate: "customer_devstation.body",
			Breadcrumb: []BreadcrumbCrumb{
				{Label: "Customers", Href: "/customers"},
				{Label: c.DisplayName, Href: "/customers/" + slug},
				{Label: "Devstation", Href: "/customers/" + slug + "/devstation", Last: true},
			},
		},
		Customer: c,
		Services: devstationServices(slug),
	}
	renderTemplate(w, "customer_devstation.html", data)
}

// renderCustomerDevstationService handles /customers/<slug>/devstation/<svc>.
func renderCustomerDevstationService(w http.ResponseWriter, r *http.Request, slug, serviceID string) {
	svc, ok := getCustomerService(slug, serviceID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	c := Customer{AccountSlug: slug, DisplayName: strings.Title(slug), Tier: "T1"}
	data := CustomerDevstationServicePageData{
		Shell: ShellArgs{
			HeaderData:   buildHeaderData(r),
			PageTitle:    c.DisplayName + " — " + serviceID,
			BodyTemplate: "customer_devstation_service.body",
			Breadcrumb: []BreadcrumbCrumb{
				{Label: "Customers", Href: "/customers"},
				{Label: c.DisplayName, Href: "/customers/" + slug},
				{Label: "Devstation", Href: "/customers/" + slug + "/devstation"},
				{Label: serviceID, Href: "/customers/" + slug + "/devstation/" + serviceID, Last: true},
			},
		},
		Customer: c,
		Service:  svc,
	}
	renderTemplate(w, "customer_devstation_service.html", data)
}

// Stubs for the other drill destinations — return 501 with breadcrumb to
// the planned page, so the navigation works end-to-end while implementations
// land per Spec 17 §8 Phasing.
func renderCustomerBundles(w http.ResponseWriter, r *http.Request, slug string) {
	drillStub(w, r, slug, "bundles", "Bundles")
}
func renderCustomerIdentity(w http.ResponseWriter, r *http.Request, slug string) {
	drillStub(w, r, slug, "identity", "Identity")
}
func renderCustomerSecrets(w http.ResponseWriter, r *http.Request, slug string) {
	drillStub(w, r, slug, "secrets", "Secrets")
}
func renderCustomerObservability(w http.ResponseWriter, r *http.Request, slug string) {
	drillStub(w, r, slug, "observability", "Observability")
}
func renderCustomerNetwork(w http.ResponseWriter, r *http.Request, slug string) {
	drillStub(w, r, slug, "network", "Network")
}

func drillStub(w http.ResponseWriter, r *http.Request, slug, segment, label string) {
	c := Customer{AccountSlug: slug, DisplayName: strings.Title(slug), Tier: "T1"}
	type stubData struct {
		Shell   ShellArgs
		Segment string
		Label   string
	}
	data := stubData{
		Shell: ShellArgs{
			HeaderData:   buildHeaderData(r),
			PageTitle:    c.DisplayName + " — " + label,
			BodyTemplate: "customer_drill_stub.body",
			Breadcrumb: []BreadcrumbCrumb{
				{Label: "Customers", Href: "/customers"},
				{Label: c.DisplayName, Href: "/customers/" + slug},
				{Label: label, Href: "/customers/" + slug + "/" + segment, Last: true},
			},
		},
		Segment: segment,
		Label:   label,
	}
	w.WriteHeader(http.StatusNotImplemented)
	renderTemplate(w, "customer_drill_stub.html", data)
}

// renderTemplate is a thin wrapper to keep handlers concise. The real
// template registration in main.go ParseFS already includes
// templates/*.html + components/*.html.
func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if templates == nil {
		log.Printf("template error: template root not initialised")
		http.Error(w, "template not ready", http.StatusInternalServerError)
		return
	}
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error: %v", err)
	}
}
