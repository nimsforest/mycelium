# Mycelium Testbook

Manual test procedures for verifying mycelium NATS auth service.

## Environment

**Where are you running these tests?**

| Environment | Hub | Spoke | How to run commands |
|-------------|-----|-------|---------------------|
| **Production** | land-shared-one (178.104.70.180) | land-nimsforest-one (46.225.164.179) | See below |

Set these variables for the rest of the testbook:

```bash
HUB=178.104.70.180      # land-shared-one
SPOKE=46.225.164.179     # land-nimsforest-one
NATS_HUB=nats://$HUB:4222
```

### Prerequisites on the tester's machine

- `nats` CLI (`go install github.com/nats-io/natscli/nats@latest`)
- `curl`
- `ssh` access to hub and spoke as root
- `base64`, `grep`, `cut` (standard shell tools)

### What runs where

- **SSH to hub/spoke**: admin ops that need localhost access (mycelium API on :8090, docker, NATS monitoring on :8222)
- **Local `nats` CLI**: all pub/sub/subscribe commands connect remotely to `$NATS_HUB` using `.creds` files
- **Local shell tools**: JWT inspection via `base64 -d`

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

**Goal**: Verify account JWTs contain correct exports/imports.

Decode the organisationland account:

```bash
ssh root@$HUB "curl -s localhost:8090/api/nats-config | grep -oP '\"organisationland\":\"\\K[^\"]+' | cut -d. -f2 | base64 -d 2>/dev/null"
```

- [ ] Shows export with subject `tap.landregistry.>`
- [ ] Shows import with subject `land.status.>` referencing the default account's public key

Decode the default account:

```bash
ssh root@$HUB "curl -s localhost:8090/api/nats-config | grep -oP '\"default\":\"\\K[^\"]+' | cut -d. -f2 | base64 -d 2>/dev/null"
```

- [ ] Shows export with subject `land.status.>`
- [ ] Shows import with subject `tap.landregistry.>` referencing the organisationland account's public key

## Test 4: Issue credentials via API

**Goal**: Verify credentials can be issued. Issue a default credential (for pub/sub tests) and a system credential (for server inspection).

Default credential:

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"testbook-hub\"}' | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g'" > /tmp/testbook-default.creds
```

- [ ] File contains `BEGIN NATS USER JWT`
- [ ] File contains `BEGIN USER NKEY SEED`

System credential (for server info, leaf node reports):

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/system \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"testbook-system\"}' | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g'" > /tmp/testbook-system.creds
```

Verify both credentials work from the tester's machine:

```bash
nats pub test.connectivity '{"from": "tester"}' --server $NATS_HUB --creds /tmp/testbook-default.creds
nats server info --server $NATS_HUB --creds /tmp/testbook-system.creds
```

- [ ] Default publish succeeds
- [ ] Server info shows server version and `Auth Required: true`

## Test 5: Issue credential via dashboard

**Goal**: Verify the web dashboard works.

1. Open via SSH tunnel: `ssh -L 8090:localhost:8090 root@$HUB`, then browse to `http://localhost:8090/dashboard/`

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

Issue a throwaway credential and extract its public key:

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"revoke-test\"}' | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g'" > /tmp/testbook-revoke.creds

head -2 /tmp/testbook-revoke.creds | tail -1 | cut -d. -f2 | base64 -d 2>/dev/null | grep -oP '"sub":"\K[^"]*'
```

Revoke it (replace `<PUBLIC_KEY>` with the output above):

```bash
ssh root@$HUB "curl -s -X DELETE localhost:8090/api/credentials/<PUBLIC_KEY>"
```

- [ ] Returns `{"status":"revoked"}`

## Test 7: Leaf node connectivity

**Goal**: Verify the spoke (land-nimsforest-one) is connected to the hub via leaf node.

```bash
nats server report leafnodes --server $NATS_HUB --creds /tmp/testbook-system.creds
```

- [ ] Shows a leaf from `46.225.164.179` (land-nimsforest-one)
- [ ] Leaf is associated with the organisationland account's public key

## Test 8: Hub auth is active

**Goal**: Verify the hub forest is using TrustedOperators auth from mycelium.

```bash
nats server info --server $NATS_HUB --creds /tmp/testbook-system.creds
```

- [ ] Shows `Auth Required: true`

## Test 9: Cross-account export/import — spoke to hub

**Goal**: Verify `tap.landregistry.>` flows from organisationland (spoke) to default (hub) via export/import.

Terminal A — subscribe from tester's machine using default account credentials:

```bash
nats sub 'tap.landregistry.>' --count 1 --server $NATS_HUB --creds /tmp/testbook-default.creds
```

Terminal B — publish from the spoke (spoke's local NATS has no auth):

```bash
ssh root@$SPOKE "nats pub tap.landregistry.lands.create '{\"test\": true}'"
```

- [ ] Terminal A receives the message on `tap.landregistry.lands.create`
- [ ] Payload matches what was sent

## Test 10: Cross-account export/import — hub to spoke

**Goal**: Verify `land.status.>` flows from default (hub) to organisationland (spoke) via export/import.

Terminal A — subscribe on the spoke (spoke's local NATS has no auth):

```bash
ssh root@$SPOKE "nats sub 'land.status.>' --count 1"
```

Terminal B — publish from tester's machine using default account credentials:

```bash
nats pub land.status.test '{"status": "ok"}' --server $NATS_HUB --creds /tmp/testbook-default.creds
```

- [ ] Terminal A on spoke receives the message
- [ ] Payload matches

## Test 11: Non-exported subjects do NOT cross accounts

**Goal**: Verify subject isolation between accounts.

Terminal A — subscribe on the spoke for a subject NOT in imports:

```bash
ssh root@$SPOKE "timeout 5 nats sub 'song.telegram.>' --count 1; echo 'TIMED OUT (expected)'"
```

Terminal B — publish from tester's machine on default account:

```bash
nats pub song.telegram.send '{"test": true}' --server $NATS_HUB --creds /tmp/testbook-default.creds
```

- [ ] Spoke subscriber times out — message does NOT cross accounts

## Test 12: Subject ACL enforcement at leaf node boundary

**Goal**: Verify that only permitted subjects cross from spoke to hub. The spoke's local NATS does not enforce per-user ACLs — enforcement happens at the leaf node boundary between accounts.

Permitted subject — subscribe from tester's machine, publish from spoke:

```bash
nats sub 'tap.landregistry.>' --count 1 --server $NATS_HUB --creds /tmp/testbook-default.creds &
sleep 2
ssh root@$SPOKE "nats pub tap.landregistry.lands.create '{\"test\": true}'"
```

- [ ] Tester receives the message (export/import allows it)

Non-permitted subject — subscribe from tester's machine, publish from spoke:

```bash
timeout 5 nats sub 'forest.land.>' --count 1 --server $NATS_HUB --creds /tmp/testbook-default.creds; echo 'TIMED OUT (expected)' &
sleep 2
ssh root@$SPOKE "nats pub forest.land.status '{\"test\": true}'"
```

- [ ] Tester does NOT receive the message (no export/import for this subject)

## Test 13: Per-credential permission overrides

**Goal**: Verify that per-credential publish/subscribe overrides narrow the effective permissions.

Issue a credential with narrower permissions than the account default:

```bash
ssh root@$HUB "curl -s -X POST localhost:8090/api/credentials/default \
  -H 'Content-Type: application/json' \
  -d '{\"name\": \"narrow-test\", \"publish\": [\"land.status.>\"], \"subscribe\": [\"land.status.>\"]}' \
  | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g'" > /tmp/testbook-narrow.creds
```

Test from the tester's machine:

```bash
# Should succeed — in the credential's publish list
nats pub land.status.test '{"ok": true}' --server $NATS_HUB --creds /tmp/testbook-narrow.creds

# Should be DENIED — not in the credential's publish list (even though account allows it)
nats pub tap.landregistry.lands.create '{"test": true}' --server $NATS_HUB --creds /tmp/testbook-narrow.creds
```

- [ ] `land.status.test` publish succeeds
- [ ] `tap.landregistry.lands.create` publish is denied (permissions violation)

Decode the credential JWT to confirm:

```bash
head -2 /tmp/testbook-narrow.creds | tail -1 | cut -d. -f2 | base64 -d 2>/dev/null
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
     | grep -oP '\"credentials\":\"\\K[^\"]*' | sed 's/\\\\n/\n/g'" > /tmp/testbook-revoke-prop.creds
   ```

2. Verify it works from the tester's machine:
   ```bash
   nats pub land.status.test '{"ok": true}' --server $NATS_HUB --creds /tmp/testbook-revoke-prop.creds
   ```
   - [ ] Publish succeeds

3. Extract public key and revoke:
   ```bash
   head -2 /tmp/testbook-revoke-prop.creds | tail -1 | cut -d. -f2 | base64 -d 2>/dev/null | grep -oP '"sub":"\K[^"]*'
   ssh root@$HUB "curl -s -X DELETE localhost:8090/api/credentials/<PUBLIC_KEY>"
   ```
   - [ ] Returns `{"status":"revoked"}`

4. Wait 60 seconds for hub auth refresh, then retry:
   ```bash
   nats pub land.status.test '{"ok": true}' --server $NATS_HUB --creds /tmp/testbook-revoke-prop.creds
   ```
   - [ ] Publish fails with authorization error

## Test 15: Full lifecycle — landregistry tap event

**Goal**: End-to-end test that a land creation request from spoke reaches landregistry on hub.

Terminal A — subscribe from tester's machine on the hub's default account:

```bash
nats sub 'tap.landregistry.>' --count 1 --server $NATS_HUB --creds /tmp/testbook-default.creds
```

Terminal B — publish from spoke:

```bash
ssh root@$SPOKE "nats pub tap.landregistry.lands.create '{\"name\":\"testbook-land\",\"organization\":\"nimsforest\"}'"
```

- [ ] Tester receives the message on `tap.landregistry.lands.create`
- [ ] Payload matches what was sent

## Cleanup

Remove local test credentials:

```bash
rm -f /tmp/testbook-default.creds /tmp/testbook-system.creds /tmp/testbook-narrow.creds /tmp/testbook-revoke.creds /tmp/testbook-revoke-prop.creds
```

Revoke test credentials via dashboard (`ssh -L 8090:localhost:8090 root@$HUB`, then browse to `http://localhost:8090/dashboard/`) — remove any credentials named `testbook-*`, `dashboard-test`, `narrow-test`, `revoke-*`.
