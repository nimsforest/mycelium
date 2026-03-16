package store

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
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
	Accounts    map[string]string `json:"accounts"` // account name → signed account JWT
}

// CredentialStore manages NATS operator, account, and user credentials.
type CredentialStore struct {
	store *Store
}

// NewCredentialStore creates a new CredentialStore.
func NewCredentialStore(s *Store) *CredentialStore {
	return &CredentialStore{store: s}
}

// Bootstrap generates operator and default account keys on first boot.
// It is idempotent — does nothing if keys already exist.
func (cs *CredentialStore) Bootstrap() error {
	// Check if operator already exists
	_, err := cs.store.Get("operator.keys")
	if err == nil {
		log.Printf("[CredentialStore] operator keys already exist, skipping bootstrap")
		return nil
	}
	if err != ErrNotFound {
		return fmt.Errorf("failed to check operator keys: %w", err)
	}

	// Generate operator
	opKP, err := nkeys.CreateOperator()
	if err != nil {
		return fmt.Errorf("failed to create operator key: %w", err)
	}
	opPub, _ := opKP.PublicKey()
	opSeed, _ := opKP.Seed()

	if err := cs.store.PutJSON("operator.keys", &KeyPair{
		PublicKey: opPub,
		Seed:      string(opSeed),
	}); err != nil {
		return fmt.Errorf("failed to store operator keys: %w", err)
	}
	log.Printf("[CredentialStore] operator bootstrapped: %s", opPub)

	// Generate default account
	if err := cs.createAccount("default"); err != nil {
		return fmt.Errorf("failed to create default account: %w", err)
	}

	return nil
}

// createAccount generates an account key pair and stores it.
func (cs *CredentialStore) createAccount(name string) error {
	accKP, err := nkeys.CreateAccount()
	if err != nil {
		return fmt.Errorf("failed to create account key: %w", err)
	}
	accPub, _ := accKP.PublicKey()
	accSeed, _ := accKP.Seed()

	if err := cs.store.PutJSON("accounts."+name+".keys", &KeyPair{
		PublicKey: accPub,
		Seed:      string(accSeed),
	}); err != nil {
		return fmt.Errorf("failed to store account keys: %w", err)
	}
	log.Printf("[CredentialStore] account bootstrapped: %s (%s)", name, accPub)
	return nil
}

// IssueCredential generates a user NKey pair, signs a JWT with the account key,
// and returns the .creds file content.
func (cs *CredentialStore) IssueCredential(name, account string) (string, error) {
	if account == "" {
		account = "default"
	}

	// Load account keys
	var accKeys KeyPair
	if err := cs.store.GetJSON("accounts."+account+".keys", &accKeys); err != nil {
		return "", fmt.Errorf("account %q not found: %w", account, err)
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

	// Create and sign user JWT
	claims := jwt.NewUserClaims(userPub)
	claims.Name = name
	claims.IssuerAccount = accKeys.PublicKey

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
	if err := cs.store.PutJSON("credentials."+userPub, cred); err != nil {
		return "", fmt.Errorf("failed to store credential: %w", err)
	}

	log.Printf("[CredentialStore] credential issued: %s (account=%s, pub=%s)", name, account, userPub)
	return string(creds), nil
}

// ListCredentials returns all issued credentials.
func (cs *CredentialStore) ListCredentials() ([]*Credential, error) {
	keys, err := cs.store.Keys()
	if err != nil {
		return nil, err
	}

	var creds []*Credential
	for _, k := range keys {
		if !strings.HasPrefix(k, "credentials.") {
			continue
		}
		var c Credential
		if err := cs.store.GetJSON(k, &c); err != nil {
			continue
		}
		creds = append(creds, &c)
	}
	return creds, nil
}

// ListAccounts returns the names of all accounts.
func (cs *CredentialStore) ListAccounts() ([]string, error) {
	keys, err := cs.store.Keys()
	if err != nil {
		return nil, err
	}

	var accounts []string
	for _, k := range keys {
		// Key format: accounts.<name>.keys
		if !strings.HasPrefix(k, "accounts.") || !strings.HasSuffix(k, ".keys") {
			continue
		}
		name := strings.TrimPrefix(k, "accounts.")
		name = strings.TrimSuffix(name, ".keys")
		accounts = append(accounts, name)
	}
	return accounts, nil
}

// RevokeCredential revokes a credential by adding it to the account's revocation
// list and removing its metadata. The revocation is embedded in the account JWT
// returned by GetNATSConfig, so NATS servers that refresh will reject the credential.
func (cs *CredentialStore) RevokeCredential(publicKey string) error {
	// Load credential to find its account
	var cred Credential
	if err := cs.store.GetJSON("credentials."+publicKey, &cred); err != nil {
		return fmt.Errorf("credential not found: %w", err)
	}

	// Add to revocation list for the account
	var revocations Revocations
	_ = cs.store.GetJSON("revocations."+cred.Account, &revocations) // may not exist yet
	revocations.Keys = append(revocations.Keys, RevokedKey{
		PublicKey: publicKey,
		RevokedAt: time.Now().UTC(),
	})
	if err := cs.store.PutJSON("revocations."+cred.Account, &revocations); err != nil {
		return fmt.Errorf("failed to store revocation: %w", err)
	}

	// Remove credential metadata
	_ = cs.store.Delete("credentials." + publicKey)

	log.Printf("[CredentialStore] credential revoked: %s (account=%s)", publicKey, cred.Account)
	return nil
}

// GetNATSConfig returns the operator JWT and account JWTs for configuring
// a NATS server with TrustedOperators + memory resolver.
func (cs *CredentialStore) GetNATSConfig() (*NATSConfig, error) {
	// Load operator keys
	var opKeys KeyPair
	if err := cs.store.GetJSON("operator.keys", &opKeys); err != nil {
		return nil, fmt.Errorf("operator not bootstrapped: %w", err)
	}
	opKP, err := nkeys.FromSeed([]byte(opKeys.Seed))
	if err != nil {
		return nil, fmt.Errorf("failed to load operator seed: %w", err)
	}

	// Create and sign operator JWT
	opClaims := jwt.NewOperatorClaims(opKeys.PublicKey)
	opClaims.Name = "mycelium"

	// Collect all account public keys as signing keys
	accounts, err := cs.ListAccounts()
	if err != nil {
		return nil, err
	}

	accountJWTs := make(map[string]string)
	for _, accName := range accounts {
		var accKeys KeyPair
		if err := cs.store.GetJSON("accounts."+accName+".keys", &accKeys); err != nil {
			continue
		}

		// Sign account JWT with operator key
		accClaims := jwt.NewAccountClaims(accKeys.PublicKey)
		accClaims.Name = accName

		// Embed revocations
		var revocations Revocations
		if err := cs.store.GetJSON("revocations."+accName, &revocations); err == nil {
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
