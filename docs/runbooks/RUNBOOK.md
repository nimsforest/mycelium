# Mycelium Runbook

## Overview

Mycelium is the central identity service for NimsForest. It manages organizations, users, platform links, and passports via a NATS JetStream KV store.

- **Server**: nimsforest (46.225.173.0)
- **JSON API**: :8090 (internal, no TLS)
- **Dashboard**: https://mycelium.nimsforest.com (autocert TLS on :443)
- **Binary**: `/usr/local/bin/mycelium`
- **Config**: `/etc/mycelium/mycelium.yaml`
- **Cert cache**: `/var/lib/mycelium/certs`
- **Systemd**: `mycelium.service`
- **User**: `mycelium:mycelium`
- **KV bucket**: `MYCELIUM_SOIL` on root NATS JetStream

## Deploy

Cross-compile, upload, restart:

```bash
cd /home/claude-user/mycelium
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=v0.X.X" -o /tmp/mycelium-linux-amd64 ./cmd/mycelium/
scp /tmp/mycelium-linux-amd64 root@46.225.173.0:/tmp/mycelium-new
ssh root@46.225.173.0 "systemctl stop mycelium && mv /tmp/mycelium-new /usr/local/bin/mycelium && chmod +x /usr/local/bin/mycelium && systemctl start mycelium"
rm /tmp/mycelium-linux-amd64
```

Verify:

```bash
ssh root@46.225.173.0 "systemctl status mycelium --no-pager"
ssh root@46.225.173.0 "curl -sS localhost:8090/health"
curl -sS https://mycelium.nimsforest.com/
```

## CLI Commands

```
mycelium serve --config /etc/mycelium/mycelium.yaml
mycelium create-organization <name> --slug <slug>
mycelium create-user <email> [--name <name>]
mycelium link-platform <user_id> <platform> <platform_id>
mycelium grant-passport <user_id> <org_slug>
mycelium provision <org_slug>
mycelium list-organizations
mycelium list-users
mycelium update
mycelium check-update
mycelium version
```

## Config

`/etc/mycelium/mycelium.yaml`:

```yaml
listen: ":8090"
nats_url: "nats://127.0.0.1:4222"
forest_server: "46.225.173.0"
land_server: "46.225.164.179"
base_nats_port: 4222
base_land_port: 8080
domain: "mycelium.nimsforest.com"
cert_dir: "/var/lib/mycelium/certs"
```

| Field | Purpose | Default |
|-------|---------|---------|
| `listen` | JSON API bind address | `:8090` |
| `nats_url` | NATS server URL | `nats://127.0.0.1:4222` |
| `forest_server` | SSH target for forest provisioning | — |
| `land_server` | SSH target for land provisioning | — |
| `base_nats_port` | Starting NATS port for organizations | `4222` |
| `base_land_port` | Starting land port for organizations | `8080` |
| `domain` | Dashboard domain (enables TLS on :443) | — |
| `cert_dir` | Let's Encrypt cert cache directory | `/var/lib/mycelium/certs` |

## Systemd Service

`/etc/systemd/system/mycelium.service`:

- Runs as `mycelium:mycelium` user
- `AmbientCapabilities=CAP_NET_BIND_SERVICE` for ports 80/443
- Auto-restarts on failure (5s delay)
- Hardened: `NoNewPrivileges`, `ProtectHome`, etc.

```bash
systemctl status mycelium
systemctl restart mycelium
journalctl -u mycelium -f
```

## Infrastructure

### DNS

`mycelium.nimsforest.com` A record → 46.225.173.0

```bash
export HCLOUD_CONTEXT=nimsforest
hcloud zone rrset list nimsforest.com --type A
```

### Firewall

Hetzner firewall `firewall-1` and UFW both allow ports 80/tcp and 443/tcp.

```bash
# UFW
ssh root@46.225.173.0 "ufw status"

# Hetzner
export HCLOUD_CONTEXT=nimsforest
hcloud firewall describe firewall-1
```

### NATS Subjects

Mycelium listens on NATS request/reply:

| Subject | Purpose |
|---------|---------|
| `mycelium.resolve.platform.<platform>` | Resolve platform ID → user |
| `mycelium.resolve.user.<user_id>` | Resolve user ID → user + organizations |
| `mycelium.resolve.passport.<agent_id>` | Check passport access for an agent |

### KV Keys

All data in `MYCELIUM_SOIL` bucket:

| Key pattern | Value |
|------------|-------|
| `organizations.<slug>` | Organization JSON |
| `users.<user_id>` | User JSON |
| `memberships.<slug>.<user_id>` | Membership JSON |
| `organization_members.<slug>` | MemberList (reverse index) |
| `user_organizations.<user_id>` | OrganizationList (reverse index) |
| `passports.<agent_id>` | Passport JSON |
| `platforms.<platform>.<platform_id>` | PlatformLink JSON |

Inspect with `nats` CLI on the nimsforest server:

```bash
ssh root@46.225.173.0
nats kv ls MYCELIUM_SOIL
nats kv get MYCELIUM_SOIL organizations.nimsforest
```

## Troubleshooting

### Dashboard not loading

1. Check service: `systemctl status mycelium`
2. Check logs: `journalctl -u mycelium --since '5 min ago'`
3. Check ports: `ss -tlnp | grep -E ':(80|443) '`
4. Check certs: `ls -la /var/lib/mycelium/certs/`
5. Check DNS: `dig mycelium.nimsforest.com`

### Permission denied on :80/:443

Ensure `AmbientCapabilities=CAP_NET_BIND_SERVICE` is in the service file:

```bash
grep AmbientCapabilities /etc/systemd/system/mycelium.service
```

### NATS connection failed

Verify NATS is running and accessible:

```bash
systemctl status nimsforest
nats server ping
```

### Auto-update

Mycelium checks for updates every 6 hours from releases.experiencenet.com. Manual update:

```bash
ssh root@46.225.173.0 "mycelium update"
```
