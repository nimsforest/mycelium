# Mycelium Testbook

Manual test procedures for verifying mycelium NATS auth service.

## Prerequisites

- mycelium binary built: `cd /home/claude-user/mycelium && go build -o mycelium ./cmd/mycelium`
- NATS server running on `127.0.0.1:4222` with JetStream enabled
- `nats` CLI installed
- `curl` and `jq` installed

## Test 1: Bootstrap and startup

**Goal**: Verify mycelium starts, connects to NATS, and bootstraps operator + account keys.

1. Create a minimal config:
   ```bash
   cat > /tmp/mycelium-test.yaml <<'EOF'
   listen: ":8090"
   nats_url: "nats://127.0.0.1:4222"
   data_dir: "/tmp/mycelium-data"
   operator_name: "nimsforest"
   accounts:
     default:
       publish: ["*"]
       subscribe: ["*"]
     nimsforest:
       publish:
         - "tap.landregistry.>"
         - "forest.land.>"
       subscribe:
         - "land.status.>"
         - "cloud.>"
   EOF
   ```

2. Start mycelium:
   ```bash
   ./mycelium serve --config /tmp/mycelium-test.yaml
   ```

3. **Verify**: Logs show:
   - [ ] `connected to NATS at nats://127.0.0.1:4222`
   - [ ] `[auth] operator bootstrapped: O...` (first run) or `operator keys already exist` (subsequent)
   - [ ] `[auth] account bootstrapped: default` (first run)
   - [ ] `[auth] account bootstrapped: nimsforest` (first run)
   - [ ] `mycelium serving on :8090`

4. **Verify idempotency**: Restart mycelium. Logs should show "already exist, skipping" for all keys.

## Test 2: Health endpoint

```bash
curl -s http://127.0.0.1:8090/health | jq .
```

- [ ] Returns `{"status": "ok", "version": "...", "service": "mycelium"}`

## Test 3: NATS config API

```bash
curl -s http://127.0.0.1:8090/api/nats-config | jq .
```

- [ ] Response contains `operator_jwt` (non-empty string starting with `ey`)
- [ ] Response contains `accounts` object with keys `default` and `nimsforest`
- [ ] Each account value is a JWT string

Decode and inspect:
```bash
curl -s http://127.0.0.1:8090/api/nats-config | jq -r '.operator_jwt' | nats jwt decode -
curl -s http://127.0.0.1:8090/api/nats-config | jq -r '.accounts.nimsforest' | nats jwt decode -
```

- [ ] Operator JWT shows `name: nimsforest`, type: operator
- [ ] nimsforest account JWT shows default permissions matching config (publish: `tap.landregistry.>`, `forest.land.>`, subscribe: `land.status.>`, `cloud.>`)

## Test 4: Issue credential via API

```bash
curl -s -X POST http://127.0.0.1:8090/api/credentials/nimsforest \
  -H 'Content-Type: application/json' \
  -d '{"name": "test-leaf"}' | jq .
```

- [ ] Response contains `credentials` field with `.creds` file content
- [ ] Content includes `-----BEGIN NATS USER JWT-----` and `-----BEGIN USER NKEY SEED-----`

## Test 5: Issue credential via dashboard

1. Open `http://127.0.0.1:8090/dashboard/` in a browser
2. **Verify index page**:
   - [ ] Shows operator status: `active`
   - [ ] Shows account count: `2`
   - [ ] Shows credential count (matches number of issued credentials)

3. Navigate to Accounts page:
   - [ ] Lists `default` and `nimsforest` accounts
   - [ ] Shows correct publish/subscribe permissions for each

4. Navigate to Credentials page:
   - [ ] Shows issue form with name input and account dropdown
   - [ ] Enter name `dashboard-test`, select `nimsforest`, click Issue
   - [ ] Credential content displayed with Download and Copy buttons
   - [ ] Credential appears in the table below

## Test 6: Revoke credential

1. From the credentials table, click "revoke" on a credential
2. Confirm the dialog
3. **Verify**:
   - [ ] Credential disappears from the table
   - [ ] `curl -s http://127.0.0.1:8090/api/nats-config | jq -r '.accounts.nimsforest' | nats jwt decode -` shows the revoked public key in revocations

Or via API:
```bash
curl -s -X DELETE http://127.0.0.1:8090/api/credentials/<PUBLIC_KEY> | jq .
```
- [ ] Returns `{"status": "revoked"}`

## Test 7: Hub NATS auth integration

**Goal**: Verify the hub forest picks up auth from mycelium.

1. On land-shared-one, ensure mycelium is running on port 8090
2. Set `MYCELIUM_URL=http://127.0.0.1:8090` in forest container environment
3. Restart the hub forest

4. **Verify**:
   ```bash
   nats server info
   ```
   - [ ] Shows accounts (not just `$G`)
   - [ ] Auth is enabled

5. Wait 60 seconds for refresh cycle, then check again:
   - [ ] Account JWTs are being refreshed (check mycelium logs for fetch requests)

## Test 8: Spoke leaf node with credentials

**Goal**: Verify spoke connects to hub using .creds file.

1. Issue a credential for the nimsforest account:
   ```bash
   curl -s -X POST http://127.0.0.1:8090/api/credentials/nimsforest \
     -H 'Content-Type: application/json' \
     -d '{"name": "nimsforest-leaf"}' | jq -r '.credentials' > /tmp/nimsforest.creds
   ```

2. SCP to spoke:
   ```bash
   scp /tmp/nimsforest.creds root@<spoke-ip>:/etc/nimsforest/nimsforest.creds
   ```

3. Update spoke forest config (`/opt/nimsforest/config/forest.yaml`):
   ```yaml
   organization:
     slug: nimsforest
     mycelium_url: nats://178.104.70.180:7422
     mycelium_credentials: /etc/nimsforest/nimsforest.creds
   ```

4. Restart spoke forest

5. **Verify**:
   - [ ] Spoke logs show `Leaf node remote configured: nats://178.104.70.180:7422 (credentials=/etc/nimsforest/nimsforest.creds)`
   - [ ] Leaf node connects successfully (no auth errors in hub logs)

## Test 9: Subject ACL enforcement

**Goal**: Verify the nimsforest account can only publish/subscribe to permitted subjects.

From the spoke (using credentials):

```bash
# Should succeed — in nimsforest publish list
nats pub tap.landregistry.lands.create '{"test": true}' --creds /etc/nimsforest/nimsforest.creds
nats pub forest.land.status '{"status": "ok"}' --creds /etc/nimsforest/nimsforest.creds

# Should succeed — in nimsforest subscribe list
nats sub land.status.test --creds /etc/nimsforest/nimsforest.creds &
nats sub cloud.provisioned --creds /etc/nimsforest/nimsforest.creds &

# Should be DENIED — not in nimsforest publish list
nats pub cloud.provisioned '{"test": true}' --creds /etc/nimsforest/nimsforest.creds
nats pub song.telegram.send '{"test": true}' --creds /etc/nimsforest/nimsforest.creds
```

- [ ] Permitted publishes succeed
- [ ] Permitted subscribes succeed
- [ ] Denied publishes return permissions violation error

## Test 10: Full lifecycle — landregistry tap event

**Goal**: End-to-end test that a land creation request from spoke reaches landregistry on hub.

1. On hub, subscribe to landregistry tap:
   ```bash
   nats sub "tap.landregistry.>" --count 1
   ```

2. From spoke, publish:
   ```bash
   nats pub tap.landregistry.lands.create '{"name":"test-land","organization":"nimsforest"}' \
     --creds /etc/nimsforest/nimsforest.creds
   ```

3. **Verify**:
   - [ ] Hub receives the message on `tap.landregistry.lands.create`
   - [ ] Message payload matches what was sent

## Test 11: Credential revocation propagation

**Goal**: Verify that revoking a credential prevents further access.

1. Issue a credential and verify it works (can publish)
2. Revoke it via API or dashboard
3. Wait 60 seconds for hub auth refresh
4. Try to publish using the revoked credential

- [ ] Publish fails with authorization error after refresh
