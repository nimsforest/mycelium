package web

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"

	nwc "github.com/nimsforest/nimsforestwebcomponents"

	"github.com/nimsforest/mycelium/internal/auth"
)

type pageData struct {
	Title   string
	Nav     string
	Data    any
	Version string
}

type Server struct {
	mux         *http.ServeMux
	templates   map[string]*template.Template
	credentials *auth.Service
	version     string
}

// NewServer creates an HTTP handler for the dashboard.
func NewServer(credentials *auth.Service, version string) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		templates:   make(map[string]*template.Template),
		credentials: credentials,
		version:     version,
	}
	srv.parseTemplates()
	srv.routes()
	return srv
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	srv.mux.ServeHTTP(w, r)
}

// StaticHandler returns the static file handler for nimsforestwebcomponents assets.
func (srv *Server) StaticHandler() http.Handler {
	return http.StripPrefix("/static/", nwc.StaticHandler())
}

func (srv *Server) parseTemplates() {
	fm := nwc.FuncMap()

	layoutBytes, err := fs.ReadFile(templateFS, "templates/layout.html")
	if err != nil {
		log.Fatalf("web: failed to read layout.html: %v", err)
	}

	pages := []string{
		"index.html",
		"accounts.html",
		"credentials.html",
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
	srv.mux.HandleFunc("GET /accounts", srv.handleAccounts)
	srv.mux.HandleFunc("GET /credentials", srv.handleCredentials)
	srv.mux.HandleFunc("POST /credentials/create", srv.handleCredentialCreate)
	srv.mux.HandleFunc("POST /credentials/{pub}/revoke", srv.handleCredentialRevoke)
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
	accounts := srv.credentials.ListAccounts()
	creds, _ := srv.credentials.ListCredentials()

	// Check operator status
	_, err := srv.credentials.GetNATSConfig()
	operatorStatus := "active"
	if err != nil {
		operatorStatus = "error"
	}

	srv.render(w, r, "index.html", pageData{
		Title: "Overview",
		Nav:   "overview",
		Data: map[string]any{
			"OperatorStatus":  operatorStatus,
			"AccountCount":    len(accounts),
			"CredentialCount": len(creds),
		},
	})
}

func (srv *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	type accountInfo struct {
		Name      string
		Publish   []string
		Subscribe []string
	}

	var accounts []accountInfo
	for _, name := range srv.credentials.ListAccounts() {
		perms, _ := srv.credentials.GetAccountPermissions(name)
		accounts = append(accounts, accountInfo{
			Name:      name,
			Publish:   perms.Publish,
			Subscribe: perms.Subscribe,
		})
	}

	srv.render(w, r, "accounts.html", pageData{
		Title: "Accounts",
		Nav:   "accounts",
		Data: map[string]any{
			"Accounts": accounts,
		},
	})
}

func (srv *Server) handleCredentials(w http.ResponseWriter, r *http.Request) {
	creds, _ := srv.credentials.ListCredentials()
	accounts := srv.credentials.ListAccounts()

	srv.render(w, r, "credentials.html", pageData{
		Title: "Credentials",
		Nav:   "credentials",
		Data: map[string]any{
			"Credentials": creds,
			"Accounts":    accounts,
		},
	})
}

func (srv *Server) handleCredentialCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	account := strings.TrimSpace(r.FormValue("account"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	credsContent, err := srv.credentials.IssueCredential(name, account, nil, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	creds, _ := srv.credentials.ListCredentials()
	accounts := srv.credentials.ListAccounts()

	srv.render(w, r, "credentials.html", pageData{
		Title: "Credentials",
		Nav:   "credentials",
		Data: map[string]any{
			"Credentials":  creds,
			"Accounts":     accounts,
			"CredsContent": credsContent,
			"CredsName":    name,
		},
	})
}

func (srv *Server) handleCredentialRevoke(w http.ResponseWriter, r *http.Request) {
	pub := r.PathValue("pub")
	if err := srv.credentials.RevokeCredential(pub); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("dashboard: credential revoked: %s", pub)
	w.Header().Set("HX-Redirect", "/dashboard/credentials")
	w.WriteHeader(http.StatusNoContent)
}
