# Security Policy

## Supported Versions

| Version | Supported |
|---------|:---------:|
| `main` (latest) | ✅ |
| < `0.9.0` | ❌ |

We only provide security fixes for the latest release. Please keep Switchblade up to date.

---

## ⚠️ Scope

Switchblade is a **self-hosted tool**. The security perimeter is your own infrastructure. Key threat surfaces:

| Surface | Risk | Notes |
|---|---|---|
| **API key exposure** | High | `API_KEY` in `.env` — restrict file permissions (`chmod 600 .env`) |
| **Credential storage** | Medium | Account passwords stored XOR+base64 encrypted. Protect `ENCRYPTION_KEY`. |
| **SQLite file access** | Medium | `data/switchblade.db` contains all credentials — restrict OS-level access |
| **Dashboard exposure** | Medium | Dashboard port (1931) should NOT be exposed to the public internet |
| **Relay tunnel** | Medium | Relay secret (`RELAY_SECRET`) must be strong and kept private |
| **Python subprocess** | Low | Auth bots run as subprocess — ensure `scripts/auth/` directory is not world-writable |

---

## Reporting a Vulnerability

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please report security issues privately:

### Option A — GitHub Private Vulnerability Reporting (Preferred)

Use GitHub's built-in **"Report a vulnerability"** button on the Security tab of this repository. This creates a private advisory visible only to maintainers.

### Option B — Email

Send a detailed report to:

```
security@[project-domain]
```

Include in your report:

- **Description** — What is the vulnerability? What is the attack vector?
- **Impact** — What can an attacker do? What data/systems are at risk?
- **Reproduction** — Step-by-step instructions to reproduce
- **Environment** — OS, Go version, Switchblade version/commit
- **Suggested fix** — (optional) If you have a fix in mind

---

## Response Timeline

| Step | Target Time |
|---|---|
| Initial acknowledgement | 48 hours |
| Severity assessment | 5 business days |
| Fix development | Depends on severity (see below) |
| Patched release | Depends on severity (see below) |
| Public disclosure | After patch is released |

### Severity SLAs

| Severity | Fix Target |
|---|---|
| 🔴 Critical (RCE, auth bypass, credential leak) | ≤ 7 days |
| 🟠 High (significant data exposure, privilege escalation) | ≤ 14 days |
| 🟡 Medium (limited impact, requires specific conditions) | ≤ 30 days |
| 🟢 Low (minimal impact, hardening) | Next release cycle |

---

## Security Best Practices for Self-Hosting

### Environment & Secrets

```bash
# Restrict .env to owner only
chmod 600 .env

# Generate a strong API key (32+ chars)
API_KEY=$(openssl rand -hex 32)

# Generate a strong encryption key (exactly 32 hex chars)
ENCRYPTION_KEY=$(openssl rand -hex 16)
```

### Network

```bash
# Bind API server to localhost only if dashboard is your primary interface
# Then reverse-proxy via nginx/caddy with TLS

# DO NOT expose port 1931 (dashboard) to the internet
# Use a VPN or SSH tunnel to access the dashboard remotely
```

### File System

```bash
# Restrict data directory
chmod 700 data/
chmod 600 data/switchblade.db

# Run switchblade as a dedicated non-root user
useradd -r -s /bin/false switchblade
chown -R switchblade:switchblade /opt/switchblade
```

### Database

- The SQLite database contains all account credentials (encrypted), API keys (bcrypt-hashed), and full request/response logs
- Back it up regularly (Switchblade does this automatically, but also keep off-machine backups)
- Do not include `data/` in any backups that are uploaded to cloud storage unless they are encrypted at rest

### Relay Tunnel

- `RELAY_SECRET` must be a high-entropy random string (32+ chars)
- If running a relay server, restrict access to known peers via firewall rules
- The relay exposes your Switchblade instance to the public internet — ensure `API_KEY` is strong

---

## Known Limitations

1. **XOR+base64 encryption** for stored credentials is obfuscation, not strong encryption. The `ENCRYPTION_KEY` must be kept secret. If your `ENCRYPTION_KEY` leaks, all stored account passwords should be considered compromised and rotated.

2. **SQLite WAL files** (`.db-shm`, `.db-wal`) may contain recent plaintext data during operation. These are included in `.gitignore` but should also be protected at the OS level.

3. **Request body logging** (`POOLPROX_LOG_BODY_ENABLED=true`) stores full request/response bodies in `request_logs`. This may include sensitive prompt content. Consider setting `POOLPROX_LOG_BODY_ENABLED=false` in production, or enabling `POOLPROX_LOG_BODY_REDACT=true`.

4. **Python subprocess** security depends on the integrity of the `scripts/auth/` directory. Do not allow untrusted code to be placed there.

---

## Acknowledgements

We thank all security researchers who responsibly disclose vulnerabilities. Confirmed reporters will be credited in the release notes (unless they prefer to remain anonymous).
