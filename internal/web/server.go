package web

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	nwc "github.com/nimsforest/nimsforestwebcomponents"

	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/pkg/mycelium"
)

type pageData struct {
	Title   string
	Nav     string
	Data    any
	Version string
}

type Server struct {
	mux           *http.ServeMux
	templates     map[string]*template.Template
	users         *store.UserStore
	organizations *store.OrganizationStore
	memberships   *store.MembershipStore
	passports     *store.PassportStore
	store         *store.Store
	version       string
}

// NewServer creates an HTTP handler for the admin dashboard.
func NewServer(s *store.Store, version string) *Server {
	srv := &Server{
		mux:           http.NewServeMux(),
		templates:     make(map[string]*template.Template),
		users:         store.NewUserStore(s),
		organizations: store.NewOrganizationStore(s),
		memberships:   store.NewMembershipStore(s),
		passports:     store.NewPassportStore(s),
		store:         s,
		version:       version,
	}
	srv.parseTemplates()
	srv.routes()
	return srv
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	srv.mux.ServeHTTP(w, r)
}

func (srv *Server) parseTemplates() {
	fm := nwc.FuncMap()

	layoutBytes, err := fs.ReadFile(templateFS, "templates/layout.html")
	if err != nil {
		log.Fatalf("web: failed to read layout.html: %v", err)
	}

	pages := []string{
		"index.html",
		"organizations.html",
		"organization-detail.html",
		"users.html",
		"user-detail.html",
		"passports.html",
	}

	for _, page := range pages {
		pageBytes, err := fs.ReadFile(templateFS, "templates/"+page)
		if err != nil {
			log.Fatalf("web: failed to read %s: %v", page, err)
		}

		t := template.New("layout.html").Funcs(fm)
		template.Must(t.Parse(string(layoutBytes)))
		template.Must(t.New(page).Parse(string(pageBytes)))

		srv.templates[page] = t
	}
}

func (srv *Server) routes() {
	srv.mux.Handle("GET /static/", http.StripPrefix("/static/", nwc.StaticHandler()))

	srv.mux.HandleFunc("GET /{$}", srv.handleIndex)
	srv.mux.HandleFunc("GET /organizations", srv.handleOrganizations)
	srv.mux.HandleFunc("GET /organizations/{slug}", srv.handleOrganizationDetail)
	srv.mux.HandleFunc("POST /organizations/create", srv.handleOrganizationCreate)
	srv.mux.HandleFunc("GET /users", srv.handleUsers)
	srv.mux.HandleFunc("GET /users/{id}", srv.handleUserDetail)
	srv.mux.HandleFunc("POST /users/create", srv.handleUserCreate)
	srv.mux.HandleFunc("POST /users/{id}/link", srv.handleUserLink)
	srv.mux.HandleFunc("POST /users/{id}/grant", srv.handleUserGrant)
	srv.mux.HandleFunc("GET /passports", srv.handlePassports)
	srv.mux.HandleFunc("POST /passports/create", srv.handlePassportCreate)
	srv.mux.HandleFunc("POST /passports/{agent_id}/delete", srv.handlePassportDelete)
}

func (srv *Server) render(w http.ResponseWriter, r *http.Request, page string, pd pageData) {
	pd.Version = srv.version
	t, ok := srv.templates[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		if err := t.ExecuteTemplate(w, "content", pd); err != nil {
			log.Printf("web: render fragment error for %s: %v", page, err)
		}
		return
	}
	if err := t.ExecuteTemplate(w, "layout.html", pd); err != nil {
		log.Printf("web: render error for %s: %v", page, err)
	}
}

// --- Page handlers ---

func (srv *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	orgs, _ := srv.organizations.List()
	users, _ := srv.users.List()
	passports, _ := srv.passports.List()

	srv.render(w, r, "index.html", pageData{
		Title: "Overview",
		Nav:   "overview",
		Data: map[string]int{
			"OrganizationCount": len(orgs),
			"UserCount":         len(users),
			"PassportCount":     len(passports),
		},
	})
}

func (srv *Server) handleOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, _ := srv.organizations.List()

	srv.render(w, r, "organizations.html", pageData{
		Title: "Organizations",
		Nav:   "organizations",
		Data: map[string]any{
			"Organizations": orgs,
		},
	})
}

func (srv *Server) handleOrganizationDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	org, err := srv.organizations.Get(slug)
	if err != nil {
		http.Redirect(w, r, "/organizations", http.StatusSeeOther)
		return
	}

	memberIDs, _ := srv.memberships.GetOrganizationMembers(slug)
	var members []*mycelium.User
	for _, id := range memberIDs {
		u, err := srv.users.Get(id)
		if err == nil {
			members = append(members, u)
		}
	}

	srv.render(w, r, "organization-detail.html", pageData{
		Title: org.Name,
		Nav:   "organizations",
		Data: map[string]any{
			"Organization": org,
			"Members":      members,
		},
	})
}

func (srv *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	users, _ := srv.users.List()

	srv.render(w, r, "users.html", pageData{
		Title: "Users",
		Nav:   "users",
		Data: map[string]any{
			"Users": users,
		},
	})
}

func (srv *Server) handleUserDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, err := srv.users.Get(id)
	if err != nil {
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}

	orgSlugs, _ := srv.memberships.GetUserOrganizations(id)
	platforms := srv.getPlatformLinks(id)
	allOrgs, _ := srv.organizations.List()

	srv.render(w, r, "user-detail.html", pageData{
		Title: user.Name,
		Nav:   "users",
		Data: map[string]any{
			"User":             user,
			"Organizations":    orgSlugs,
			"Platforms":        platforms,
			"AllOrganizations": allOrgs,
		},
	})
}

var defaultCapabilities = []string{"read", "write", "admin", "deploy", "observe"}

func (srv *Server) handlePassports(w http.ResponseWriter, r *http.Request) {
	passports, _ := srv.passports.List()
	orgs, _ := srv.organizations.List()
	users, _ := srv.users.List()

	srv.render(w, r, "passports.html", pageData{
		Title: "Passports",
		Nav:   "passports",
		Data: map[string]any{
			"Passports":     passports,
			"Organizations": orgs,
			"Users":         users,
			"Capabilities":  defaultCapabilities,
		},
	})
}

// --- Form handlers ---

func (srv *Server) handleOrganizationCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	if name == "" || slug == "" {
		http.Error(w, "name and slug are required", http.StatusBadRequest)
		return
	}

	org := &mycelium.Organization{
		Slug:      slug,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if err := srv.organizations.Create(org); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("dashboard: organization planted: %s (%s)", name, slug)
	w.Header().Set("HX-Redirect", "/organizations/"+slug)
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	name := strings.TrimSpace(r.FormValue("name"))
	if email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}

	user := &mycelium.User{
		ID:        store.GenerateUserID(),
		Email:     email,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if err := srv.users.Create(user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("dashboard: user sprouted: %s (%s)", email, user.ID)
	w.Header().Set("HX-Redirect", "/users/"+user.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handleUserLink(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	platform := strings.TrimSpace(r.FormValue("platform"))
	platformID := strings.TrimSpace(r.FormValue("platform_id"))
	if platform == "" || platformID == "" {
		http.Error(w, "platform and platform_id are required", http.StatusBadRequest)
		return
	}

	link := mycelium.PlatformLink{UserID: id}
	key := fmt.Sprintf("platforms.%s.%s", platform, platformID)
	if err := srv.store.PutJSON(key, &link); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("dashboard: platform linked: %s.%s → %s", platform, platformID, id)
	w.Header().Set("HX-Redirect", "/users/"+id)
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handleUserGrant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	orgSlug := strings.TrimSpace(r.FormValue("organization_slug"))
	role := strings.TrimSpace(r.FormValue("role"))
	if orgSlug == "" {
		http.Error(w, "organization is required", http.StatusBadRequest)
		return
	}
	if role == "" {
		role = "member"
	}

	m := &mycelium.Membership{
		UserID:           id,
		OrganizationSlug: orgSlug,
		Role:             role,
		JoinedAt:         time.Now().UTC(),
	}
	if err := srv.memberships.Grant(m); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("dashboard: membership granted: %s → %s (%s)", id, orgSlug, role)
	w.Header().Set("HX-Redirect", "/users/"+id)
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handlePassportCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	// If a user was selected, use their ID and set type to human
	userID := strings.TrimSpace(r.FormValue("user_id"))
	agentID := strings.TrimSpace(r.FormValue("agent_id"))
	agentType := strings.TrimSpace(r.FormValue("agent_type"))

	if userID != "" {
		agentID = userID
		agentType = "human"
	}

	if agentID == "" {
		http.Error(w, "select a user or enter an agent ID", http.StatusBadRequest)
		return
	}
	if agentType == "" {
		agentType = "service"
	}

	p := &mycelium.Passport{
		AgentID:              agentID,
		AgentType:            agentType,
		HomeOrganization:     strings.TrimSpace(r.FormValue("home_organization")),
		AllowedOrganizations: r.Form["allowed_organizations"],
		Capabilities:         r.Form["capabilities"],
	}
	if err := srv.passports.Create(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("dashboard: passport issued: %s (%s)", agentID, agentType)
	w.Header().Set("HX-Redirect", "/passports")
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handlePassportDelete(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	if err := srv.store.Delete("passports." + agentID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("dashboard: passport revoked: %s", agentID)
	w.Header().Set("HX-Redirect", "/passports")
	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

type platformEntry struct {
	Platform   string
	PlatformID string
}

func (srv *Server) getPlatformLinks(userID string) []platformEntry {
	keys, err := srv.store.Keys()
	if err != nil {
		return nil
	}
	var entries []platformEntry
	for _, k := range keys {
		if !strings.HasPrefix(k, "platforms.") {
			continue
		}
		var link mycelium.PlatformLink
		if err := srv.store.GetJSON(k, &link); err != nil {
			continue
		}
		if link.UserID != userID {
			continue
		}
		// key format: platforms.<platform>.<platform_id>
		parts := strings.SplitN(k, ".", 3)
		if len(parts) == 3 {
			entries = append(entries, platformEntry{
				Platform:   parts[1],
				PlatformID: parts[2],
			})
		}
	}
	return entries
}
