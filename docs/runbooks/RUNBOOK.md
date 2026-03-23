# Mycelium Runbook

## Overview

Mycelium is the NATS TrustedOperators auth service for NimsForest. It manages operator and account NKeys, issues user credentials (.creds files), and serves NATS auth config (JWTs) for hub and spoke forest servers.

- **Server**: land-shared-one (land-shared-one.nimsforest.com)
- **Deployment**: Docker container `mycelium`, host networking
- **HTTP API**: port 8090
- **Dashboard**: port 8090 at `/dashboard/`
- **Config**: `/opt/mycelium/config.yaml` (bind-mounted)
- **Data**: `/var/lib/mycelium/` (bind-mounted, contains keys + credentials)
- **KV bucket**: `MYCELIUM_SOIL` on NATS JetStream

## Deploy

Cross-compile, upload, rebuild container, restart:

```bash
cd /home/claude-user/mycelium
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=v0.X.X" -o /tmp/mycelium-linux-amd64 ./cmd/mycelium/
scp /tmp/mycelium-linux-amd64 root@land-shared-one.nimsforest.com:/opt/mycelium/mycelium
ssh root@land-shared-one.nimsforest.com "cd /opt/mycelium && docker build -t mycelium . && docker rm -f mycelium && docker run -d --name mycelium --network host -v /opt/mycelium/config.yaml:/etc/mycelium/config.yaml:ro -v /var/lib/mycelium:/var/lib/mycelium mycelium"
rm /tmp/mycelium-linux-amd64
```

Verify:

```bash
ssh root@land-shared-one.nimsforest.com "docker logs --tail 20 mycelium"
ssh root@land-shared-one.nimsforest.com "curl -s localhost:8090/health"
```

## CLI Commands

```
mycelium serve --config /etc/mycelium/config.yaml
mycelium version
```

## Config

`/opt/mycelium/config.yaml` (on land-shared-one):

```yaml
listen: ":8090"
nats_url: "nats://127.0.0.1:4222"
data_dir: "/var/lib/mycelium"
operator_name: "nimsforest"
accounts:
  hub:
    publish: [">"]
    subscribe: [">"]
    exports:
      - name: land-status
        subject: "land.status.>"
        type: stream
    imports:
      - name: org-provisioning
        subject: "tap.landregistry.>"
        account: organisationland
        type: stream
  system:
    publish: [">"]
    subscribe: [">"]
  nimsforest:
    publish:
      - "tap.landregistry.>"
      - "forest.land.>"
    subscribe:
      - "land.status.>"
      - "cloud.>"
  organisationland:
    publish:
      - "tap.landregistry.lands.create"
      - "tap.landregistry.lands.*.delete"
      - "_INBOX.>"
    subscribe:
      - "land.status.>"
      - "_INBOX.>"
    exports:
      - name: org-provisioning
        subject: "tap.landregistry.>"
        type: stream
    imports:
      - name: land-status
        subject: "land.status.>"
        account: hub
        type: stream
```

| Field | Purpose | Default |
|-------|---------|---------|
| `listen` | HTTP API bind address | `:8090` |
| `nats_url` | NATS server URL | `nats://127.0.0.1:4222` |
| `data_dir` | Persistent data directory (keys, credentials) | `/var/lib/mycelium` |
| `operator_name` | Name for the NATS operator JWT | `nimsforest` |
| `accounts` | Map of account name → permissions (publish, subscribe, exports, imports) | single `hub` account |

## KV Keys

All data in `MYCELIUM_SOIL` bucket:

| Key pattern | Value |
|------------|-------|
| `operator.keys` | Operator KeyPair (public_key, seed) |
| `accounts.<name>.keys` | Account KeyPair (public_key, seed) |
| `credentials.<public_key>` | Credential metadata (name, account, public_key, created_at) |
| `revocations.<account>` | Revocation list (array of revoked public keys + timestamps) |

Note: bare `nats` CLI on land-shared-one requires auth. Use credentials or access NATS via mycelium API.

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/health` | Health check |
| `GET` | `/api/nats-config` | Get operator + account JWTs for NATS resolver |
| `POST` | `/api/credentials/{account}` | Issue a new credential |
| `DELETE` | `/api/credentials/{publickey}` | Revoke a credential (also publishes NATS event) |
| `GET` | `/dashboard/` | Web dashboard |

## Troubleshooting

### Container not starting

```bash
ssh root@land-shared-one.nimsforest.com "docker ps -a | grep mycelium"
ssh root@land-shared-one.nimsforest.com "docker logs mycelium"
```

### NATS connection failed

NATS is embedded in the forest container on land-shared-one:

```bash
ssh root@land-shared-one.nimsforest.com "docker ps | grep nimsforest"
ssh root@land-shared-one.nimsforest.com "ss -tlnp | grep 4222"
```

### Dashboard not loading

Check mycelium is running and port is open:

```bash
ssh root@land-shared-one.nimsforest.com "curl -s localhost:8090/health"
ssh root@land-shared-one.nimsforest.com "ss -tlnp | grep 8090"
```

### Auth refresh after credential changes

Mycelium publishes `mycelium.auth.updated` on NATS after every credential issue or revoke. The forest (nimsforest2) subscribes to this event and immediately re-fetches account JWTs from `/api/nats-config`, then hot-reloads the NATS resolver. Revocations propagate in ~1-2 seconds.

If the NATS subscription is not established (e.g., during bootstrap), the forest falls back to polling every 60 seconds, with a 5-minute safety-net ticker when the subscription is active.

### Keys lost after container recreate

Keys are persisted in two places: NATS KV (`MYCELIUM_SOIL`) and on disk (`/var/lib/mycelium/keys/`). As long as either the NATS data or the bind-mounted data directory survives, keys are recovered automatically on startup.
