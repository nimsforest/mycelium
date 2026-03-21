# Mycelium Testbook

Manual test procedures for verifying mycelium NATS auth service.

## Environment

**Where are you running these tests?**

| Environment | Hub | Spoke | How to run commands |
|-------------|-----|-------|---------------------|
| **Production** | land-shared-one (178.104.70.180) | land-nimsforest-one (46.225.164.179) | `ssh root@<ip> "<command>"` |

Set these variables for the rest of the testbook:

```bash
HUB=178.104.70.180      # land-shared-one
SPOKE=46.225.164.179     # land-nimsforest-one
```

### Available tools on production servers

- `curl` — HTTP requests
- `nats` — NATS CLI (requires credentials for authenticated NATS)
- `grep`, `awk`, `sed`, `cut` — text processing
- `docker` — container management

## Test 1: Mycelium health

**Goal**: Verify mycelium is running on the hub.

```bash
ssh root@$HUB "curl -s localhost:8090/health"
```

- [ ] Returns JSON with `"status":"ok"` and `"service":"mycelium"`

```bash
ssh root@$HUB "docker ps --format '{{.Names}} {{.Status}}' | grep mycelium"
```

- [ ] Shows `mycelium Up ...`

## Test 2: NATS config API

**Goal**: Verify mycelium serves operator + account JWTs.

```bash
ssh root@$HUB "curl -s localhost:8090/api/nats-config | grep -o '\"operator_jwt\":\"ey' | head -1"
```

- [ ] Output contains `"operator_jwt":"ey` (JWT present, starts with `ey`)

```bash
ssh root@$HUB "curl -s localhost:8090/api/nats-config | grep -oE '\"(default|nimsforest|organisationland|system)\":\"ey' | sort"
```

- [ ] Shows all four accounts: `default`, `nimsforest`, `organisationland`, `system`

## Test 3: Account JWT contents

**Goal**: Verify account JWTs contain correct permissions and exports/imports.

Decode the organisationland account to check exports/imports:

```bash
ssh root@$HUB "curl -s localhost:8090/api/nats-config | grep -oP '\"organisationland\":\"\\K[^\"]+' | nats jwt decode -"
```

- [ ] Shows exports for `tap.landregistry.>`
- [ ] Shows imports for `land.status.>` from the default account's public key

Decode the default account:

```bash
ssh root@$HUB "curl -s localhost:8090/api/nats-config | grep -oP '\"default\":\"\\K[^\"]+' | nats jwt decode -"
```

- [ ] Shows exports for `land.status.>`
- [ ] Shows imports for `tap.landregistry.>` from the organisationland account's public key

## Test 4: Issue credential via API

**Goal**: Verify credentials can be issued.

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"testbook-default\"}'"
```

- [ ] Response contains `BEGIN NATS USER JWT`
- [ ] Response contains `BEGIN USER NKEY SEED`

Save a default credential for later tests:

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"testbook-hub\"}' | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g' > /tmp/default.creds"
```

## Test 5: Issue credential via dashboard

**Goal**: Verify the web dashboard works.

1. Open `http://178.104.70.180:8090/dashboard/` in a browser (or via SSH tunnel: `ssh -L 8090:localhost:8090 root@178.104.70.180`)

2. **Verify index page**:
   - [ ] Shows operator status: `active`
   - [ ] Shows account count: `4`
   - [ ] Shows credential count

3. Navigate to Accounts page:
   - [ ] Lists `default`, `nimsforest`, `organisationland`, `system`
   - [ ] Shows correct publish/subscribe permissions for each

4. Navigate to Credentials page:
   - [ ] Shows issue form with name input and account dropdown
   - [ ] Enter name `dashboard-test`, select `organisationland`, click Issue
   - [ ] Credential content displayed with Download and Copy buttons
   - [ ] Credential appears in the table below

## Test 6: Revoke credential

**Goal**: Verify credential revocation works.

First, issue a throwaway credential:

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"revoke-test\"}'"
```

Extract the public key from the response (look for the `U` key in the credentials field), then revoke it:

```bash
ssh root@$HUB "curl -s -X DELETE localhost:8090/api/credentials/<PUBLIC_KEY>"
```

- [ ] Returns `{"status":"revoked"}`

Or use the dashboard: click "revoke" next to the credential in the Credentials page.

## Test 7: Leaf node connectivity

**Goal**: Verify the spoke (land-nimsforest-one) is connected to the hub via leaf node.

```bash
ssh root@$HUB "curl -s http://127.0.0.1:8222/leafz"
```

- [ ] Shows a leaf from `46.225.164.179` (land-nimsforest-one)
- [ ] Leaf is associated with an account (the organisationland account's public key)

## Test 8: Hub auth is active

**Goal**: Verify the hub forest is using TrustedOperators auth from mycelium.

```bash
ssh root@$HUB "curl -s http://127.0.0.1:8222/varz | grep auth_required"
```

- [ ] Shows `"auth_required": true`

## Test 9: Cross-account export/import — spoke to hub

**Goal**: Verify `tap.landregistry.>` flows from organisationland (spoke) to default (hub) via export/import.

Terminal A — subscribe on the hub (using default account credentials):

```bash
ssh root@$HUB "nats sub 'tap.landregistry.>' --count 1 --creds /tmp/default.creds"
```

Terminal B — publish from the spoke:

```bash
ssh root@$SPOKE "nats pub tap.landregistry.lands.create '{\"test\": true}'"
```

- [ ] Terminal A receives the message on `tap.landregistry.lands.create`
- [ ] Payload matches what was sent

## Test 10: Cross-account export/import — hub to spoke

**Goal**: Verify `land.status.>` flows from default (hub) to organisationland (spoke) via export/import.

Terminal A — subscribe on the spoke:

```bash
ssh root@$SPOKE "nats sub 'land.status.>' --count 1"
```

Terminal B — publish from the hub (using default account credentials):

```bash
ssh root@$HUB "nats pub land.status.test '{\"status\": \"ok\"}' --creds /tmp/default.creds"
```

- [ ] Terminal A on spoke receives the message
- [ ] Payload matches

## Test 11: Non-exported subjects do NOT cross accounts

**Goal**: Verify subject isolation between accounts.

Terminal A — subscribe on the spoke for a subject NOT in imports:

```bash
ssh root@$SPOKE "timeout 5 nats sub 'song.telegram.>' --count 1; echo 'TIMED OUT (expected)'"
```

Terminal B — publish from the hub on default account:

```bash
ssh root@$HUB "nats pub song.telegram.send '{\"test\": true}' --creds /tmp/default.creds"
```

- [ ] Spoke subscriber times out — message does NOT cross accounts

## Test 12: Subject ACL enforcement on spoke

**Goal**: Verify the organisationland account can only publish/subscribe to permitted subjects.

From the spoke:

```bash
# Should succeed — in organisationland publish list
ssh root@$SPOKE "nats pub tap.landregistry.lands.create '{\"test\": true}' 2>&1"

# Should be DENIED — not in organisationland publish list
ssh root@$SPOKE "nats pub forest.land.status '{\"test\": true}' 2>&1"
ssh root@$SPOKE "nats pub song.telegram.send '{\"test\": true}' 2>&1"
```

- [ ] `tap.landregistry.lands.create` publish succeeds
- [ ] `forest.land.status` publish is denied (permissions violation)
- [ ] `song.telegram.send` publish is denied (permissions violation)

## Test 13: Per-credential permission overrides

**Goal**: Verify that per-credential publish/subscribe overrides narrow the effective permissions.

Issue a credential with narrower permissions than the account default:

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"narrow-test\", \"publish\": [\"land.status.>\"], \"subscribe\": [\"land.status.>\"]}' \
  | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g' > /tmp/narrow.creds"
```

Test with the narrowed credential:

```bash
# Should succeed — in the credential's publish list
ssh root@$HUB "nats pub land.status.test '{\"ok\": true}' --creds /tmp/narrow.creds 2>&1"

# Should be DENIED — not in the credential's publish list (even though account allows it)
ssh root@$HUB "nats pub tap.landregistry.lands.create '{\"test\": true}' --creds /tmp/narrow.creds 2>&1"
```

- [ ] `land.status.test` publish succeeds
- [ ] `tap.landregistry.lands.create` publish is denied

Decode the credential JWT to confirm:

```bash
ssh root@$HUB "cat /tmp/narrow.creds | nats jwt decode -"
```

- [ ] Pub allow list contains only `_INBOX.>` and `land.status.>`
- [ ] Sub allow list contains only `_INBOX.>` and `land.status.>`

## Test 14: Credential revocation propagation

**Goal**: Verify that revoking a credential prevents further access.

1. Issue a credential for default account:
   ```bash
   ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
     -H 'Content-Type: application/json' \
     -d '{\"name\": \"revoke-prop-test\"}' \
     | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g' > /tmp/revoke-test.creds"
   ```

2. Verify it works:
   ```bash
   ssh root@$HUB "nats pub land.status.test '{\"ok\": true}' --creds /tmp/revoke-test.creds"
   ```
   - [ ] Publish succeeds

3. Extract public key and revoke:
   ```bash
   ssh root@$HUB "cat /tmp/revoke-test.creds | nats jwt decode - 2>&1 | grep 'Subject:'"
   ssh root@$HUB "curl -s -X DELETE localhost:8090/api/credentials/<PUBLIC_KEY>"
   ```
   - [ ] Returns `{"status":"revoked"}`

4. Wait 60 seconds for hub auth refresh, then retry:
   ```bash
   ssh root@$HUB "nats pub land.status.test '{\"ok\": true}' --creds /tmp/revoke-test.creds"
   ```
   - [ ] Publish fails with authorization error

## Test 15: Full lifecycle — landregistry tap event

**Goal**: End-to-end test that a land creation request from spoke reaches landregistry on hub.

Terminal A — on hub, watch landregistry logs:

```bash
ssh root@$HUB "docker logs -f landregistry 2>&1 | grep -i 'tap.landregistry'"
```

Terminal B — from spoke, publish:

```bash
ssh root@$SPOKE "nats pub tap.landregistry.lands.create '{\"name\":\"testbook-land\",\"organization\":\"nimsforest\"}'"
```

- [ ] Hub landregistry logs show the received message
- [ ] Payload matches what was sent

## Cleanup

Remove test credentials created during this testbook:

```bash
ssh root@$HUB "rm -f /tmp/default.creds /tmp/narrow.creds /tmp/revoke-test.creds"
```

Revoke test credentials via dashboard at `http://178.104.70.180:8090/dashboard/` — remove any credentials named `testbook-*`, `dashboard-test`, `narrow-test`, `revoke-prop-test`.
