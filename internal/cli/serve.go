package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nimsforest/mycelium/internal/auth"
	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/internal/web"
	"github.com/spf13/cobra"
)

func serveCmd(version string) *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the mycelium NATS auth service",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}

			// Ensure data dir exists
			credsDir := filepath.Join(cfg.DataDir, "credentials")
			os.MkdirAll(credsDir, 0700)
			selfCredsPath := filepath.Join(credsDir, "mycelium.creds")

			// Connect to NATS (use own credentials if available, for reconnecting after auth restart)
			connectOpts := []nats.Option{
				nats.Name("mycelium"),
				nats.MaxReconnects(-1),
				nats.ReconnectWait(time.Second),
			}
			if _, err := os.Stat(selfCredsPath); err == nil {
				connectOpts = append(connectOpts, nats.UserCredentials(selfCredsPath))
				log.Printf("using credentials: %s", selfCredsPath)
			}

			nc, err := nats.Connect(cfg.NATSURL, connectOpts...)
			if err != nil {
				return fmt.Errorf("failed to connect to NATS at %s: %w", cfg.NATSURL, err)
			}
			defer nc.Close()
			log.Printf("connected to NATS at %s", cfg.NATSURL)

			js, err := nc.JetStream()
			if err != nil {
				return fmt.Errorf("failed to get JetStream context: %w", err)
			}

			s, err := store.New(js)
			if err != nil {
				return fmt.Errorf("failed to create store: %w", err)
			}

			// Convert config accounts to auth permissions
			accountPerms := make(map[string]auth.AccountPermissions)
			for name, ap := range cfg.Accounts {
				perms := auth.AccountPermissions{
					Publish:   ap.Publish,
					Subscribe: ap.Subscribe,
				}
				for _, exp := range ap.Exports {
					perms.Exports = append(perms.Exports, auth.ExportPermission{
						Name:    exp.Name,
						Subject: exp.Subject,
						Type:    exp.Type,
					})
				}
				for _, imp := range ap.Imports {
					perms.Imports = append(perms.Imports, auth.ImportPermission{
						Name:    imp.Name,
						Subject: imp.Subject,
						Account: imp.Account,
						Type:    imp.Type,
					})
				}
				accountPerms[name] = perms
			}

			// Bootstrap NATS credentials
			keysDir := filepath.Join(cfg.DataDir, "keys")
			credentials := auth.NewService(s, keysDir, cfg.OperatorName, accountPerms)
			if err := credentials.Bootstrap(); err != nil {
				return fmt.Errorf("failed to bootstrap credentials: %w", err)
			}

			// Bootstrap mycelium's own credential for surviving auth restarts
			if _, err := os.Stat(selfCredsPath); os.IsNotExist(err) {
				credsContent, err := credentials.IssueCredential("mycelium", "default", nil, nil)
				if err != nil {
					log.Printf("warning: failed to issue self credential: %v", err)
				} else {
					if err := os.WriteFile(selfCredsPath, []byte(credsContent), 0600); err != nil {
						log.Printf("warning: failed to write self credential: %v", err)
					} else {
						log.Printf("self credential bootstrapped: %s", selfCredsPath)
					}
				}
			}

			// HTTP server
			mux := http.NewServeMux()

			// Health
			mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, map[string]any{
					"status":  "ok",
					"version": version,
					"service": "mycelium",
				})
			})

			// NATS auth config API
			mux.HandleFunc("GET /api/nats-config", func(w http.ResponseWriter, r *http.Request) {
				natsCfg, err := credentials.GetNATSConfig()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, natsCfg)
			})

			// Issue credential
			mux.HandleFunc("POST /api/credentials/{account}", func(w http.ResponseWriter, r *http.Request) {
				account := r.PathValue("account")
				var req struct {
					Name      string   `json:"name"`
					Publish   []string `json:"publish,omitempty"`
					Subscribe []string `json:"subscribe,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
					return
				}
				if req.Name == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
					return
				}

				credsContent, err := credentials.IssueCredential(req.Name, account, req.Publish, req.Subscribe)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusCreated, map[string]string{"credentials": credsContent})
			})

			// Revoke credential
			mux.HandleFunc("DELETE /api/credentials/{publickey}", func(w http.ResponseWriter, r *http.Request) {
				publicKey := r.PathValue("publickey")
				if err := credentials.RevokeCredential(publicKey); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
			})

			// Dashboard
			dashboard := web.NewServer(credentials, version)
			mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard", dashboard))
			mux.Handle("POST /dashboard/", http.StripPrefix("/dashboard", dashboard))
			mux.Handle("GET /static/", dashboard.StaticHandler())

			srv := &http.Server{
				Addr:         cfg.Listen,
				Handler:      mux,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  120 * time.Second,
			}

			go func() {
				log.Printf("mycelium serving on %s", cfg.Listen)
				log.Printf("  API:       %s/api/nats-config", cfg.Listen)
				log.Printf("  Dashboard: %s/dashboard/", cfg.Listen)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("server error: %v", err)
				}
			}()

			_, cancel := context.WithCancel(context.Background())
			defer cancel()

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

	cmd.Flags().StringVar(&configPath, "config", "/etc/mycelium/config.yaml", "path to config file")

	return cmd
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
