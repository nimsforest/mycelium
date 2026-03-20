package auth

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/nimsforest/mycelium/internal/store"
)

// KeyPair holds a public key and seed for an NKey.
type KeyPair struct {
	PublicKey string `json:"public_key"`
	Seed      string `json:"seed"`
}

// Credential represents an issued NATS credential.
type Credential struct {
	Name      string    `json:"name"`
	Account   string    `json:"account"`
	PublicKey string    `json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
}

// RevokedKey records a revoked user public key.
type RevokedKey struct {
	PublicKey string    `json:"public_key"`
	RevokedAt time.Time `json:"revoked_at"`
}

// Revocations holds the revocation list for an account.
type Revocations struct {
	Keys []RevokedKey `json:"keys"`
}

// NATSConfig is returned by the API for forest servers to load auth config.
type NATSConfig struct {
	OperatorJWT string            `json:"operator_jwt"`
	Accounts    map[string]string `json:"accounts"` // account name -> signed account JWT
}

// AccountPermissions defines subject permissions for an account (from config).
type AccountPermissions struct {
	Publish   []string
	Subscribe []string
}

// Service manages NATS operator, account, and user credentials.
type Service struct {
	store        *store.Store
	operatorName string
	accounts     map[string]AccountPermissions
}

// NewService creates a new credential service.
func NewService(s *store.Store, operatorName string, accounts map[string]AccountPermissions) *Service {
	return &Service{
		store:        s,
		operatorName: operatorName,
		accounts:     accounts,
	}
}

// Bootstrap generates operator and account NKeys from config, stores in NATS KV.
// Generates one user .creds per account (for leaf node auth). Idempotent.
func (svc *Service) Bootstrap() error {
	// Ensure operator keys exist
	_, err := svc.store.Get("operator.keys")
	if err == store.ErrNotFound {
		opKP, err := nkeys.CreateOperator()
		if err != nil {
			return fmt.Errorf("failed to create operator key: %w", err)
		}
		opPub, _ := opKP.PublicKey()
		opSeed, _ := opKP.Seed()

		if err := svc.store.PutJSON("operator.keys", &KeyPair{
			PublicKey: opPub,
			Seed:      string(opSeed),
		}); err != nil {
			return fmt.Errorf("failed to store operator keys: %w", err)
		}
		log.Printf("[auth] operator bootstrapped: %s", opPub)
	} else if err != nil {
		return fmt.Errorf("failed to check operator keys: %w", err)
	} else {
		log.Printf("[auth] operator keys already exist, skipping")
	}

	// Ensure each configured account has keys
	for name := range svc.accounts {
		key := "accounts." + name + ".keys"
		_, err := svc.store.Get(key)
		if err == store.ErrNotFound {
			if err := svc.createAccount(name); err != nil {
				return fmt.Errorf("failed to create account %s: %w", name, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check account %s: %w", name, err)
		} else {
			log.Printf("[auth] account %s keys already exist, skipping", name)
		}
	}

	return nil
}

// createAccount generates an account key pair and stores it.
func (svc *Service) createAccount(name string) error {
	accKP, err := nkeys.CreateAccount()
	if err != nil {
		return fmt.Errorf("failed to create account key: %w", err)
	}
	accPub, _ := accKP.PublicKey()
	accSeed, _ := accKP.Seed()

	if err := svc.store.PutJSON("accounts."+name+".keys", &KeyPair{
		PublicKey: accPub,
		Seed:      string(accSeed),
	}); err != nil {
		return fmt.Errorf("failed to store account keys: %w", err)
	}
	log.Printf("[auth] account bootstrapped: %s (%s)", name, accPub)
	return nil
}

// IssueCredential generates a user NKey pair, signs a JWT with the account key,
// and returns the .creds file content.
func (svc *Service) IssueCredential(name, account string) (string, error) {
	if account == "" {
		account = "default"
	}

	// Verify account exists in config
	perms, ok := svc.accounts[account]
	if !ok {
		return "", fmt.Errorf("account %q not found in config", account)
	}

	// Load account keys
	var accKeys KeyPair
	if err := svc.store.GetJSON("accounts."+account+".keys", &accKeys); err != nil {
		return "", fmt.Errorf("account %q keys not found: %w", account, err)
	}
	accKP, err := nkeys.FromSeed([]byte(accKeys.Seed))
	if err != nil {
		return "", fmt.Errorf("failed to load account seed: %w", err)
	}

	// Generate user key pair
	userKP, err := nkeys.CreateUser()
	if err != nil {
		return "", fmt.Errorf("failed to create user key: %w", err)
	}
	userPub, _ := userKP.PublicKey()
	userSeed, _ := userKP.Seed()

	// Build permissions from config
	var pubAllow, subAllow []string
	pubAllow = append(pubAllow, "_INBOX.>")
	subAllow = append(subAllow, "_INBOX.>")
	pubAllow = append(pubAllow, perms.Publish...)
	subAllow = append(subAllow, perms.Subscribe...)

	// Create and sign user JWT
	claims := jwt.NewUserClaims(userPub)
	claims.Name = name
	claims.IssuerAccount = accKeys.PublicKey
	claims.Permissions.Pub.Allow.Add(pubAllow...)
	claims.Permissions.Sub.Allow.Add(subAllow...)

	userJWT, err := claims.Encode(accKP)
	if err != nil {
		return "", fmt.Errorf("failed to sign user JWT: %w", err)
	}

	// Format .creds file
	creds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		return "", fmt.Errorf("failed to format creds: %w", err)
	}

	// Store credential metadata
	cred := &Credential{
		Name:      name,
		Account:   account,
		PublicKey: userPub,
		CreatedAt: time.Now().UTC(),
	}
	if err := svc.store.PutJSON("credentials."+userPub, cred); err != nil {
		return "", fmt.Errorf("failed to store credential: %w", err)
	}

	log.Printf("[auth] credential issued: %s (account=%s, pub=%s)", name, account, userPub)
	return string(creds), nil
}

// ListCredentials returns all issued credentials.
func (svc *Service) ListCredentials() ([]*Credential, error) {
	keys, err := svc.store.Keys()
	if err != nil {
		return nil, err
	}

	var creds []*Credential
	for _, k := range keys {
		if !strings.HasPrefix(k, "credentials.") {
			continue
		}
		var c Credential
		if err := svc.store.GetJSON(k, &c); err != nil {
			continue
		}
		creds = append(creds, &c)
	}
	return creds, nil
}

// ListAccounts returns the names of all accounts from config.
func (svc *Service) ListAccounts() []string {
	var names []string
	for name := range svc.accounts {
		names = append(names, name)
	}
	return names
}

// GetAccountPermissions returns the configured permissions for an account.
func (svc *Service) GetAccountPermissions(name string) (AccountPermissions, bool) {
	perms, ok := svc.accounts[name]
	return perms, ok
}

// RevokeCredential revokes a credential by adding it to the account's revocation
// list and removing its metadata.
func (svc *Service) RevokeCredential(publicKey string) error {
	var cred Credential
	if err := svc.store.GetJSON("credentials."+publicKey, &cred); err != nil {
		return fmt.Errorf("credential not found: %w", err)
	}

	var revocations Revocations
	_ = svc.store.GetJSON("revocations."+cred.Account, &revocations)
	revocations.Keys = append(revocations.Keys, RevokedKey{
		PublicKey: publicKey,
		RevokedAt: time.Now().UTC(),
	})
	if err := svc.store.PutJSON("revocations."+cred.Account, &revocations); err != nil {
		return fmt.Errorf("failed to store revocation: %w", err)
	}

	_ = svc.store.Delete("credentials." + publicKey)

	log.Printf("[auth] credential revoked: %s (account=%s)", publicKey, cred.Account)
	return nil
}

// GetNATSConfig returns the operator JWT and account JWTs for configuring
// a NATS server with TrustedOperators + memory resolver.
func (svc *Service) GetNATSConfig() (*NATSConfig, error) {
	// Load operator keys
	var opKeys KeyPair
	if err := svc.store.GetJSON("operator.keys", &opKeys); err != nil {
		return nil, fmt.Errorf("operator not bootstrapped: %w", err)
	}
	opKP, err := nkeys.FromSeed([]byte(opKeys.Seed))
	if err != nil {
		return nil, fmt.Errorf("failed to load operator seed: %w", err)
	}

	// Create operator JWT
	opClaims := jwt.NewOperatorClaims(opKeys.PublicKey)
	opClaims.Name = svc.operatorName

	// Build account JWTs
	accountJWTs := make(map[string]string)
	for accName, perms := range svc.accounts {
		var accKeys KeyPair
		if err := svc.store.GetJSON("accounts."+accName+".keys", &accKeys); err != nil {
			continue
		}

		accClaims := jwt.NewAccountClaims(accKeys.PublicKey)
		accClaims.Name = accName

		// Set account-level permissions (applied to all users in this account)
		accClaims.DefaultPermissions.Pub.Allow.Add(perms.Publish...)
		accClaims.DefaultPermissions.Sub.Allow.Add(perms.Subscribe...)
		accClaims.DefaultPermissions.Pub.Allow.Add("_INBOX.>")
		accClaims.DefaultPermissions.Sub.Allow.Add("_INBOX.>")

		// Enable JetStream for all accounts except the system account (-1 = unlimited).
		// Required when the NATS server runs with TrustedOperators.
		// The system account is used for internal NATS plumbing and must NOT have JetStream.
		if accName != "system" {
			accClaims.Limits.JetStreamLimits = jwt.JetStreamLimits{
				MemoryStorage: -1,
				DiskStorage:   -1,
				Streams:       -1,
				Consumer:      -1,
				MaxAckPending: -1,
			}
		}

		// Embed revocations
		var revocations Revocations
		if err := svc.store.GetJSON("revocations."+accName, &revocations); err == nil {
			for _, rk := range revocations.Keys {
				accClaims.Revoke(rk.PublicKey)
			}
		}

		accJWT, err := accClaims.Encode(opKP)
		if err != nil {
			continue
		}
		accountJWTs[accName] = accJWT
	}

	opJWT, err := opClaims.Encode(opKP)
	if err != nil {
		return nil, fmt.Errorf("failed to sign operator JWT: %w", err)
	}

	return &NATSConfig{
		OperatorJWT: opJWT,
		Accounts:    accountJWTs,
	}, nil
}
