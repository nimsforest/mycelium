package cli

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/cederikdotcom/hydrarelease/pkg/updater"
	"github.com/nimsforest/mycelium/internal/identity"
	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/internal/web"
	"github.com/nimsforest/mycelium/pkg/mycelium"
	"github.com/spf13/cobra"
)

func serveCmd(version string) *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the mycelium identity service daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}

			// Auto-update
			u := updater.NewProductionUpdater("mycelium", version)
			u.SetServiceName("mycelium")
			u.StartAutoCheck(6*time.Hour, true)
			log.Printf("auto-update: enabled (every 6h)")

			// Connect to NATS
			nc, s, err := connectAndStore(cfg.NATSURL)
			if err != nil {
				return err
			}
			defer nc.Close()
			log.Printf("connected to NATS at %s", cfg.NATSURL)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start identity resolver
			resolver := identity.NewResolver(nc, s)
			if err := resolver.Start(ctx); err != nil {
				return fmt.Errorf("failed to start resolver: %w", err)
			}
			defer resolver.Stop()

			// HTTP API
			users := store.NewUserStore(s)
			organizations := store.NewOrganizationStore(s)
			memberships := store.NewMembershipStore(s)
			passports := store.NewPassportStore(s)

			mux := http.NewServeMux()

			mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, map[string]any{
					"status":  "ok",
					"version": version,
					"service": "mycelium",
				})
			})

			// Organizations
			mux.HandleFunc("GET /organizations", func(w http.ResponseWriter, r *http.Request) {
				list, err := organizations.List()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"organizations": list})
			})

			mux.HandleFunc("POST /organizations", func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Slug string `json:"slug"`
					Name string `json:"name"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
					return
				}
				if req.Slug == "" || req.Name == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug and name are required"})
					return
				}
				org := &mycelium.Organization{
					Slug:      req.Slug,
					Name:      req.Name,
					CreatedAt: time.Now().UTC(),
				}
				if err := organizations.Create(org); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusCreated, org)
			})

			mux.HandleFunc("GET /organizations/{slug}", func(w http.ResponseWriter, r *http.Request) {
				slug := r.PathValue("slug")
				org, err := organizations.Get(slug)
				if err != nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
					return
				}
				writeJSON(w, http.StatusOK, org)
			})

			mux.HandleFunc("POST /organizations/{slug}/provision", func(w http.ResponseWriter, r *http.Request) {
				// Placeholder for provisioning
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "provisioning not yet implemented"})
			})

			// Users
			mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
				list, err := users.List()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"users": list})
			})

			mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Email string `json:"email"`
					Name  string `json:"name"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
					return
				}
				if req.Email == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
					return
				}
				user := &mycelium.User{
					ID:        store.GenerateUserID(),
					Email:     req.Email,
					Name:      req.Name,
					CreatedAt: time.Now().UTC(),
				}
				if err := users.Create(user); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusCreated, user)
			})

			mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
				id := r.PathValue("id")
				user, err := users.Get(id)
				if err != nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
					return
				}
				writeJSON(w, http.StatusOK, user)
			})

			mux.HandleFunc("POST /users/{id}/platforms", func(w http.ResponseWriter, r *http.Request) {
				id := r.PathValue("id")
				// Verify user exists
				if _, err := users.Get(id); err != nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
					return
				}
				var req struct {
					Platform   string `json:"platform"`
					PlatformID string `json:"platform_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
					return
				}
				if req.Platform == "" || req.PlatformID == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform and platform_id are required"})
					return
				}
				link := mycelium.PlatformLink{UserID: id}
				key := fmt.Sprintf("platforms.%s.%s", req.Platform, req.PlatformID)
				if err := s.PutJSON(key, &link); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusCreated, map[string]string{
					"user_id":     id,
					"platform":    req.Platform,
					"platform_id": req.PlatformID,
				})
			})

			mux.HandleFunc("POST /users/{id}/memberships", func(w http.ResponseWriter, r *http.Request) {
				id := r.PathValue("id")
				if _, err := users.Get(id); err != nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
					return
				}
				var req struct {
					OrganizationSlug string `json:"organization_slug"`
					Role             string `json:"role"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
					return
				}
				if req.OrganizationSlug == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "organization_slug is required"})
					return
				}
				if req.Role == "" {
					req.Role = "member"
				}
				m := &mycelium.Membership{
					UserID:           id,
					OrganizationSlug: req.OrganizationSlug,
					Role:             req.Role,
					JoinedAt:         time.Now().UTC(),
				}
				if err := memberships.Grant(m); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusCreated, m)
			})

			// Passports
			mux.HandleFunc("GET /passports", func(w http.ResponseWriter, r *http.Request) {
				list, err := passports.List()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"passports": list})
			})

			mux.HandleFunc("POST /passports", func(w http.ResponseWriter, r *http.Request) {
				var p mycelium.Passport
				if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
					return
				}
				if p.AgentID == "" || p.AgentType == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id and agent_type are required"})
					return
				}
				if err := passports.Create(&p); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusCreated, &p)
			})

			srv := &http.Server{
				Addr:         cfg.Listen,
				Handler:      mux,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  120 * time.Second,
			}

			go func() {
				log.Printf("mycelium serving on %s", cfg.Listen)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("server error: %v", err)
				}
			}()

			// Dashboard (autocert TLS)
			if cfg.Domain != "" {
				webSrv := web.NewServer(s, version)

				certDir := cfg.CertDir
				if certDir == "" {
					certDir = "/var/lib/mycelium/certs"
				}

				manager := &autocert.Manager{
					Prompt:     autocert.AcceptTOS,
					Cache:      autocert.DirCache(certDir),
					HostPolicy: autocert.HostWhitelist(cfg.Domain),
				}

				go func() {
					if err := http.ListenAndServe(":80", manager.HTTPHandler(nil)); err != nil {
						log.Printf("ACME HTTP server error: %v", err)
					}
				}()

				tlsSrv := &http.Server{
					Addr:    ":443",
					Handler: webSrv,
					TLSConfig: &tls.Config{
						GetCertificate: manager.GetCertificate,
					},
					ReadTimeout:  10 * time.Second,
					WriteTimeout: 30 * time.Second,
					IdleTimeout:  120 * time.Second,
				}
				go func() {
					log.Printf("dashboard at https://%s", cfg.Domain)
					if err := tlsSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
						log.Printf("TLS server error: %v", err)
					}
				}()
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			<-sigCh

			log.Println("shutting down...")
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			return srv.Shutdown(shutdownCtx)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "/etc/mycelium/mycelium.yaml", "path to config file")

	return cmd
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
