# E2E Testbook: Organization Provisioning

End-to-end test for provisioning a new organization land server from the hub, verifying the full lifecycle, and tearing it down.

## Prerequisites

- Land-shared-one (178.104.70.180) running:
  - nimsforest (forest + embedded NATS)
  - mycelium (port 8090)
  - landregistry (port 8096)
  - hetznertreehouse (NATS-connected)
- `hcloud` CLI configured with nimsforest context
- `nats` CLI installed
- Admin token for landregistry: `425a3e5b896bee36f648965ed12e6df0405baebf926e4b4341a79704bc8771fd`

## Test Variables

```bash
export HUB=178.104.70.180
export ADMIN_TOKEN="425a3e5b896bee36f648965ed12e6df0405baebf926e4b4341a79704bc8771fd"
export TEST_ORG="e2etest"
export TEST_ORG_NAME="E2E Test Org"
```

## Phase 1: Pre-flight Checks

### 1.1 Verify services are healthy

```bash
curl -s http://$HUB:8090/health | jq .
curl -s http://$HUB:8096/health | jq .
```

- [ ] Mycelium returns `{"status":"ok"}`
- [ ] Landregistry returns health response

### 1.2 Verify NATS connectivity

```bash
ssh root@$HUB "docker logs hetznertreehouse 2>&1 | tail -5"
```

- [ ] Hetznertreehouse is running and connected to NATS

### 1.3 Clean up any leftover test resources

```bash
export HCLOUD_CONTEXT=nimsforest
hcloud server list --selector org=$TEST_ORG
```

- [ ] No existing servers with label `org=e2etest`
- [ ] If servers exist, delete them: `hcloud server delete <id>`

### 1.4 Check test org doesn't exist in landregistry

```bash
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://$HUB:8096/api/v1/lands/$TEST_ORG | jq .
```

- [ ] Returns 404 or empty (org doesn't exist yet)

## Phase 2: Trigger Provisioning

### 2.1 Subscribe to status events (in a separate terminal)

```bash
nats sub "land.status.$TEST_ORG" --server nats://$HUB:4222 &
nats sub "cloud.>" --server nats://$HUB:4222 &
```

### 2.2 Create the land

```bash
curl -s -X POST http://$HUB:8096/api/v1/lands \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"org_slug\":\"$TEST_ORG\",\"org_name\":\"$TEST_ORG_NAME\"}" | jq .
```

- [ ] Returns 201 with land object
- [ ] Status is `pending`
- [ ] `land.status.e2etest` event received on NATS

### 2.3 Verify provisioning starts

```bash
# Watch hetznertreehouse logs
ssh root@$HUB "docker logs -f hetznertreehouse 2>&1" &
```

- [ ] Hetznertreehouse receives `tap.hetzner.provision`
- [ ] `cloud.provisioning` event fired (contains `server_ip` and `server_id`)
- [ ] Landregistry status updated to `provisioning`

```bash
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://$HUB:8096/api/v1/lands/$TEST_ORG | jq .
```

- [ ] Status is `provisioning`
- [ ] `server_ip` is populated

## Phase 3: Wait for Provisioning (~3-5 min)

### 3.1 Monitor progress

```bash
# Poll land status every 30 seconds
while true; do
  STATUS=$(curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
    http://$HUB:8096/api/v1/lands/$TEST_ORG | jq -r '.status')
  echo "$(date +%H:%M:%S) status=$STATUS"
  [ "$STATUS" = "active" ] && break
  [ "$STATUS" = "failed" ] && echo "PROVISIONING FAILED" && break
  sleep 30
done
```

- [ ] Status transitions: `pending` -> `provisioning` -> `active`
- [ ] `cloud.provisioned` event received on NATS

### 3.2 Verify Hetzner resources

```bash
export HCLOUD_CONTEXT=nimsforest
hcloud server list --selector org=$TEST_ORG
```

- [ ] Server exists with name `land-e2etest-one`
- [ ] Server is running
- [ ] Has public IPv4

### 3.3 Verify DNS records

```bash
export HCLOUD_CONTEXT=nimsforest
hcloud zone rrset list mynimsforest.com --type A | grep $TEST_ORG
```

- [ ] A record: `e2etest.mynimsforest.com` -> server IP
- [ ] Wildcard: `*.e2etest.mynimsforest.com` -> server IP

### 3.4 Verify server is healthy

```bash
SERVER_IP=$(curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://$HUB:8096/api/v1/lands/$TEST_ORG | jq -r '.server_ip')
curl -s http://$SERVER_IP:8080/health | jq .
```

- [ ] Server responds with health status

### 3.5 Verify landregistry record

```bash
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://$HUB:8096/api/v1/lands/$TEST_ORG | jq .
```

- [ ] Status: `active`
- [ ] `server_ip` matches Hetzner server IP
- [ ] `server_id` is populated
- [ ] `org_slug`: `e2etest`
- [ ] `org_name`: `E2E Test Org`

## Phase 4: Verify Running Land

### 4.1 Check leaf node connection

```bash
curl -s http://$HUB:8222/leafz | jq '.leafs[] | select(.name | contains("e2etest"))'
```

- [ ] Leaf node from the new land is connected to hub

### 4.2 Check land services

```bash
# Check if the land server is serving its forest
curl -s https://$TEST_ORG.mynimsforest.com/ 2>/dev/null | head -5
```

- [ ] Returns HTML content (or connection established)

## Phase 5: Teardown

### 5.1 Delete the land

```bash
curl -s -X DELETE \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://$HUB:8096/api/v1/lands/$TEST_ORG | jq .
```

- [ ] Returns `{"status":"deleting"}`
- [ ] `land.status.e2etest` event shows `deleting`

### 5.2 Wait for teardown (~30s)

```bash
sleep 10
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://$HUB:8096/api/v1/lands/$TEST_ORG | jq .
```

- [ ] Status is `deleted`
- [ ] `cloud.deleted` event received on NATS

### 5.3 Verify Hetzner cleanup

```bash
export HCLOUD_CONTEXT=nimsforest
hcloud server list --selector org=$TEST_ORG
```

- [ ] No servers with label `org=e2etest`

### 5.4 Verify DNS cleanup

```bash
export HCLOUD_CONTEXT=nimsforest
hcloud zone rrset list mynimsforest.com --type A | grep $TEST_ORG
```

- [ ] No A records for `e2etest.mynimsforest.com`
- [ ] No wildcard records

### 5.5 Kill background NATS subscribers

```bash
kill %1 %2 2>/dev/null
```

## Failure Recovery

If the test fails partway through, clean up manually:

```bash
# Delete from landregistry
curl -s -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://$HUB:8096/api/v1/lands/$TEST_ORG

# Delete Hetzner server
export HCLOUD_CONTEXT=nimsforest
hcloud server list --selector org=$TEST_ORG
hcloud server delete <server-id>

# Delete DNS records
hcloud zone rrset delete --name $TEST_ORG --type A mynimsforest.com
hcloud zone rrset delete --name "*.$TEST_ORG" --type A mynimsforest.com
```

## Expected Timeline

| Phase | Duration |
|-------|----------|
| Pre-flight | ~10s |
| Trigger provisioning | ~5s |
| Server creation | ~30-60s |
| Cloud-init + boot | ~60-120s |
| Health check pass | ~30-60s |
| Teardown | ~10-30s |
| **Total** | **~3-5 min** |
