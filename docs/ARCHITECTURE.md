# Mycelium Architecture

Mycelium is the NATS authorization service for NimsForest. It manages the cryptographic credentials that control who can publish and subscribe to what on NATS.

Mycelium handles **authorization** — what you can do. For **identity** (who you are), see Pantheon.

## NATS TrustedOperators model

NATS supports a JWT-based security model called TrustedOperators. The hierarchy is:

```
Operator (root of trust)
  └─ Account (permission boundary)
       └─ User credential (.creds file)
```

Mycelium generates and manages all three levels:

1. **Operator** — one per deployment, created on first boot. Signs account JWTs.
2. **Accounts** — defined in `config.yaml`. Each account has its own publish/subscribe permissions and can export/import subjects to share with other accounts.
3. **Credentials** — issued on demand via API or dashboard. A `.creds` file contains a user JWT (signed by the account key) and a user NKey seed.

## Account design

Four accounts serve different trust levels:

| Account | Who uses it | Permissions |
|---------|-------------|-------------|
| `default` | Services on land-shared-one (hub) | Full pub/sub (`>`) |
| `system` | NATS server internals | Full pub/sub |
| `nimsforest` | Forest-specific operations | `tap.landregistry.>`, `forest.land.>`, `land.status.>`, `cloud.>` |
| `organisationland` | Spoke lands (org forests) | `tap.landregistry.*` (create/delete), `land.status.>`, `_INBOX.>` |

### Cross-account exports/imports

Accounts are isolated by default. To share subjects between accounts, use exports (provider) and imports (consumer):

```
default account                    organisationland account
─────────────────                 ──────────────────────────
exports:                          exports:
  land.status.>  ──────────────→    (imported by organisationland)
                                    tap.landregistry.>  ──────→  (imported by default)
imports:                          imports:
  tap.landregistry.> ←──────────    land.status.> ←────────────
```

This allows spoke lands to send provisioning requests (`tap.landregistry.lands.create`) to the hub, and the hub to broadcast land status updates back.

### Per-credential permission overrides

When issuing a credential, you can specify narrower publish/subscribe permissions than the account default. For example, a credential in the `default` account could be restricted to only `land.status.>`:

```bash
curl -X POST localhost:8090/api/credentials/default \
  -d '{"name": "status-only", "publish": ["land.status.>"], "subscribe": ["land.status.>"]}'
```

The effective permissions are the intersection of account permissions and credential permissions.

## Credential lifecycle

```
Issue                              Use                              Revoke
─────                              ───                              ──────
POST /api/credentials/{account}    nim.Connect() reads .creds       DELETE /api/credentials/{key}
  → generates NKey pair            NATS validates JWT signature       → adds key to revocation list
  → signs user JWT with            NATS checks account permissions    → publishes mycelium.auth.updated
    account key                    NATS checks credential overrides   → forest re-fetches account JWTs
  → returns .creds content                                           → NATS rejects credential
```

## Push-based auth refresh

When auth changes (credential revoked), mycelium publishes a notification on NATS. The forest subscribes and immediately re-fetches the updated account JWTs:

```
Admin revokes credential
  → mycelium stores revocation in KV
  → mycelium publishes mycelium.auth.updated on NATS
  → forest receives event via internal NATS subscription
  → forest fetches /api/nats-config (JWTs now contain revocation list)
  → forest calls UpdateAccountClaims to update NATS server in-place
  → NATS rejects revoked credential on next connection attempt
```

Propagation time: ~1-2 seconds.

The forest also runs a 5-minute fallback ticker as a safety net in case a NATS event is missed. If the NATS subscription cannot be established at all (e.g., during bootstrap), it falls back to 60-second polling.

## Storage

All data lives in the `MYCELIUM_SOIL` KV bucket on NATS JetStream:

| Key pattern | Value |
|------------|-------|
| `operator.keys` | Operator NKey pair (public key + seed) |
| `accounts.<name>.keys` | Account NKey pair |
| `credentials.<public_key>` | Credential metadata (name, account, created_at) |
| `revocations.<account>` | Revocation list (revoked public keys + timestamps) |

Using JetStream KV means credentials survive container restarts without a separate database. The forest also caches auth to disk (`auth-cache.json`) so it can start with TrustedOperators even if mycelium is temporarily unavailable.

## Relationship to Pantheon

Mycelium and Pantheon are independent services:

- **Pantheon** answers "who is this person?" — users, organizations, memberships, platform links
- **Mycelium** answers "what can this NATS credential do?" — account permissions, subject ACLs, revocation

They do not call each other's APIs. Both connect to the same NATS infrastructure. A future integration could have Pantheon request mycelium credentials on behalf of authenticated users, but today credentials are issued manually to services.

See `nimsforest2/docs/architecture/IDENTITY_AND_AUTHORIZATION.md` for the full picture.
