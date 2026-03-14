// Package identity provides the NATS request/reply identity resolution service.
package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// Resolver handles identity resolution requests via NATS.
type Resolver struct {
	nc          *nats.Conn
	users       *store.UserStore
	memberships *store.MembershipStore
	passports   *store.PassportStore
	store       *store.Store
	subs        []*nats.Subscription
}

// NewResolver creates a new identity resolver.
func NewResolver(nc *nats.Conn, s *store.Store) *Resolver {
	return &Resolver{
		nc:          nc,
		users:       store.NewUserStore(s),
		memberships: store.NewMembershipStore(s),
		passports:   store.NewPassportStore(s),
		store:       s,
	}
}

// Start begins listening for identity resolution requests.
func (r *Resolver) Start(_ context.Context) error {
	// Platform resolution: mycelium.resolve.platform.<platform>
	sub1, err := r.nc.Subscribe("mycelium.resolve.platform.*", r.handlePlatformResolve)
	if err != nil {
		return fmt.Errorf("failed to subscribe to platform resolve: %w", err)
	}
	r.subs = append(r.subs, sub1)

	// User resolution: mycelium.resolve.user.*
	sub2, err := r.nc.Subscribe("mycelium.resolve.user.*", r.handleUserResolve)
	if err != nil {
		return fmt.Errorf("failed to subscribe to user resolve: %w", err)
	}
	r.subs = append(r.subs, sub2)

	// Passport resolution: mycelium.resolve.passport.*
	sub3, err := r.nc.Subscribe("mycelium.resolve.passport.*", r.handlePassportResolve)
	if err != nil {
		return fmt.Errorf("failed to subscribe to passport resolve: %w", err)
	}
	r.subs = append(r.subs, sub3)

	log.Printf("[Resolver] Listening on mycelium.resolve.>")
	return nil
}

// Stop unsubscribes from all subjects.
func (r *Resolver) Stop() {
	for _, sub := range r.subs {
		sub.Unsubscribe()
	}
	log.Printf("[Resolver] Stopped")
}

type platformRequest struct {
	PlatformID string `json:"platform_id"`
}

func (r *Resolver) handlePlatformResolve(msg *nats.Msg) {
	// Extract platform from subject: mycelium.resolve.platform.<platform>
	platform := msg.Subject[len("mycelium.resolve.platform."):]

	var req platformRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		r.replyError(msg, "invalid request")
		return
	}

	// Look up platform link
	key := fmt.Sprintf("platforms.%s.%s", platform, req.PlatformID)
	var link mycelium.PlatformLink
	if err := r.store.GetJSON(key, &link); err != nil {
		r.replyError(msg, "unknown platform identity")
		return
	}

	// Get user
	user, err := r.users.Get(link.UserID)
	if err != nil {
		r.replyError(msg, "user not found")
		return
	}

	// Get organizations
	organizations, _ := r.memberships.GetUserOrganizations(user.ID)

	// Get role from first organization membership
	role := "member"
	if len(organizations) > 0 {
		if m, err := r.memberships.GetMembership(organizations[0], user.ID); err == nil {
			role = m.Role
		}
	}

	resp := mycelium.ResolveResponse{
		UserID:        user.ID,
		Name:          user.Name,
		Email:         user.Email,
		Organizations: organizations,
		Role:          role,
	}

	data, _ := json.Marshal(resp)
	msg.Respond(data)
}

type userRequest struct {
	UserID string `json:"user_id"`
}

func (r *Resolver) handleUserResolve(msg *nats.Msg) {
	// Extract user_id from subject
	userID := msg.Subject[len("mycelium.resolve.user."):]

	// Also try from body
	var req userRequest
	if err := json.Unmarshal(msg.Data, &req); err == nil && req.UserID != "" {
		userID = req.UserID
	}

	user, err := r.users.Get(userID)
	if err != nil {
		r.replyError(msg, "user not found")
		return
	}

	organizations, _ := r.memberships.GetUserOrganizations(user.ID)

	resp := mycelium.ResolveResponse{
		UserID:        user.ID,
		Name:          user.Name,
		Email:         user.Email,
		Organizations: organizations,
	}

	data, _ := json.Marshal(resp)
	msg.Respond(data)
}

type passportRequest struct {
	AgentID                string `json:"agent_id"`
	RequestingOrganization string `json:"requesting_organization"`
}

func (r *Resolver) handlePassportResolve(msg *nats.Msg) {
	agentID := msg.Subject[len("mycelium.resolve.passport."):]

	var req passportRequest
	if err := json.Unmarshal(msg.Data, &req); err == nil && req.AgentID != "" {
		agentID = req.AgentID
	}

	passport, err := r.passports.Get(agentID)
	if err != nil {
		r.replyError(msg, "passport not found")
		return
	}

	allowed := false
	if req.RequestingOrganization != "" {
		for _, org := range passport.AllowedOrganizations {
			if org == req.RequestingOrganization || org == "*" {
				allowed = true
				break
			}
		}
	}

	resp := mycelium.PassportResponse{
		Allowed:      allowed,
		AgentType:    passport.AgentType,
		Capabilities: passport.Capabilities,
	}

	data, _ := json.Marshal(resp)
	msg.Respond(data)
}

func (r *Resolver) replyError(msg *nats.Msg, errMsg string) {
	data, _ := json.Marshal(map[string]string{"error": errMsg})
	msg.Respond(data)
}
