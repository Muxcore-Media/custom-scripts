# MuxCore Deployment Methods

This document catalogs every deployment method available for MuxCore — both existing and planned.
Each entry includes what it provides, where the files live, and its maintenance status.

---

## Legend

| Badge | Meaning |
|-------|---------|
| ✅ **Active** | Maintained by core contributors. CI-tested. Ships with releases. |
| 🟡 **Best-effort** | Exists in repo but maintained by community. May lag. PRs welcome. |
| 📋 **Planned** | Not yet implemented. Tracked in deployment roadmap. |

---

## Existing Methods

### 1. ✅ Binary Tarball (GoReleaser)

**Files:** `.goreleaser.yaml`
**Directory:** root of repo

**What it provides:**
- Prebuilt `muxcored` binaries for `linux` + `darwin` on `amd64` + `arm64`
- Released as `.tar.gz` archives with checksums on every `v*` tag
- Cosign-signed artifacts with SPDX SBOMs

**Usage:**
```bash
# Download latest
curl -LO https://github.com/Muxcore-Media/core/releases/latest/download/muxcored_Linux_x86_64.tar.gz
tar -xzf muxcored_Linux_x86_64.tar.gz
sudo install muxcored /usr/local/bin/

# Or build from source
go install github.com/Muxcore-Media/core/cmd/muxcored@latest
```

**CI:** GoReleaser runs on tag push via `.github/workflows/release.yml`.

---

### 2. ✅ systemd Unit

**Files:** `deploy/systemd/muxcored.service`, `deploy/systemd/muxcore.conf`
**Directory:** `deploy/systemd/`

**What it provides:**
- Production systemd service with comprehensive security hardening:
  - `NoNewPrivileges=true`, `ProtectSystem=strict`, `PrivateTmp=true`
  - `MemoryDenyWriteExecute=true`, `SystemCallFilter=@system-service`
  - `CapabilityBoundingSet=` (empty caps)
  - Read-write only for `/var/lib/muxcore` and `/var/log/muxcore`
- SIGHUP reload for config changes
- Restart on failure with 5s cooldown
- Environment file at `/etc/muxcore/muxcore.conf`

**Usage:**
```bash
sudo cp deploy/systemd/muxcored.service /etc/systemd/system/
sudo cp deploy/systemd/muxcore.conf /etc/muxcore/
sudo systemctl daemon-reload
sudo systemctl enable --now muxcored
```

**CI:** Not separately CI-tested (unit file is static).

---

### 3. ✅ Docker

**Files:** `Dockerfile`, `.dockerignore`
**Directory:** root of repo

**What it provides:**
- Multi-stage Docker build:
  - **Stage 1:** `golang:1.26` — compiles static binary with `CGO_ENABLED=0`
  - **Stage 2:** `alpine` — distroless runtime with `muxcore` user
- Read-only root filesystem (writable dirs at `/app/data`, `/app/tmp`)
- HEALTHCHECK on `/health`
- Exposes port 8080
- Version injected via `--build-arg VERSION`

**Usage:**
```bash
docker build --build-arg VERSION=$(git describe --tags) -t muxcore .
docker run -d --restart unless-stopped -p 127.0.0.1:8080:8080 muxcore
```

**CI:** `make docker-build` is available; not part of CI pipeline.

---

### 4. ✅ Docker Compose

**Files:** `docker-compose.yml`
**Directory:** root of repo

**What it provides:**
- Single-service compose file wrapping the Docker image
- Resource limits: 1 CPU, 512 MB RAM
- `no-new-privileges`, dropped all capabilities, `NET_BIND_SERVICE` added
- Named volume for persistent data, tmpfs for `/tmp`
- Read-only root filesystem
- Bound to localhost only (`127.0.0.1:8080`)

**Usage:**
```bash
docker compose up -d
```

**CI:** Used for dev workflow (`make dev`).

---

## Planned Methods

### 5. ✅ SSH Deploy Script

**Files:** `deploy/ssh-deploy.sh`

**What it provides:**
- Single bash script — zero dependencies beyond bash, ssh, and scp
- Pushes a locally-built binary (or downloads a release) to a remote host
- Stops the old service, replaces the binary, restarts
- Optionally copies config, TLS certs, cosign public key

**Usage:**
```bash
./deploy/ssh-deploy.sh user@host --tag default
./deploy/ssh-deploy.sh user@host --release v1.2.3 --config muxcore.json
```

**Maintenance:** 🟡 Active — maintained by core contributors.

**When to use:** Quick "push to prod" without Ansible or Docker. Works on any Linux.

---
### 6. ✅ Helm Chart

**Files:** `deploy/helm/muxcore/`
**Resources:** ServiceAccount, Secret, ConfigMap, PVC, Service, Deployment, HPA, Ingress, ServiceMonitor, test pod

**What it provides:**
- Parameterized chart with 10 templates covering the full Kubernetes deployment surface
- `Deployment` — security context, health probes, resource limits, read-only rootfs, tmpfs
- `Service` — ClusterIP (configurable to NodePort/LoadBalancer)
- `ConfigMap` — inline muxcore.json injected as file
- `Secret` — TLS certs and cluster join token from values
- `Ingress` — optional, with TLS and configurable annotations
- `PersistentVolumeClaim` — configurable size, storage class, or existing claim
- `HorizontalPodAutoscaler` — CPU/memory based scaling
- `ServiceMonitor` — Prometheus operator integration
- `ServiceAccount` — configurable, with optional annotations
- Connection test pod — `helm test` validates `/health` endpoint
- 30+ configurable values via `--set` and `--values`

**CI:** `helm lint` + `helm template` smoke tests on PR.

**Maintenance:** ✅ Active — maintained by core contributors.

**When to use:** Kubernetes deployments. Enables GitOps (ArgoCD, Flux), rolling updates, multi-node clusters. Covers everything from dev single-pod to prod with HPA, TLS, and monitoring.


---

### 7. ✅ Kustomize

**Files:** `deploy/kustomize/`
**Directories:**
- `base/` — Deployment, Service, ConfigMap, PVC
- `overlays/production/` — 2 replicas, json logging, topology spread
- `overlays/development/` — debug logging, smaller resources

**CI:** `kustomize build` smoke tests on PR.

**What it provides:**
- `base/` — Deployment, Service, ConfigMap, PVC (same shape as Helm but static YAML)
- `overlays/production/` — resource limits, replica counts, prod TLS
- `overlays/development/` — single replica, self-signed certs

**CI:** `kustomize build` smoke tests on PR.

**Maintenance:** 🟡 Active — maintained by core contributors.

**When to use:** Lightweight alternative to Helm. Pure `kubectl apply -k`. Preferred by users who vendor everything in a monorepo.

**Relationship to Helm:** Overlays complement the Helm chart; they serve different UX needs (static YAML vs parameterized chart).

---

### 8. ✅ Ansible Role

**Files:** `deploy/ansible/`
**Playbook:** `deploy/ansible/playbooks/deploy-muxcore.yml`
**Role:** `deploy/ansible/roles/muxcore/`

**What it provides:**
- Ansible role with three tagged task stages (`muxcore_install`, `muxcore_configure`, `muxcore_service`)
- Installs binary from GitHub release (specific version or `latest`) or copies a local binary
- Creates `muxcore` user, group, and directory structure
- Deploys systemd unit with full security hardening (matches `deploy/systemd/muxcored.service`)
- Writes environment config from role variables (`muxcore.conf`)
- Optionally deploys inline `muxcore.json` config via `muxcore_config_json`
- Configures logrotate for audit logs
- Opens firewall ports via firewalld or ufw (optional)
- Waits for health endpoint after deployment
- Supported platforms: Debian, Ubuntu, Fedora, RHEL

**CI:** `ansible-playbook --syntax-check` on PR.

**Maintenance:** 🟡 Active — maintained by core contributors.

**When to use:** Reproducible, idempotent deployments across many hosts. Standard for homelab automation.

---

### 9. 📋 Nomad Job Spec

**Target directory:** `deploy/nomad/`

**What it provides:**
- Nomad job file (`muxcore.nomad`) for Docker or `raw_exec` workload
- Health check via `service` + `check` stanza
- Volume mounts for audit logs and module cache
- Resource limits
- Optional Consul Connect integration

**Maintenance:** ⚠️ Best-effort. Targets Nomad 1.7+. May drift.

**When to use:** You run Nomad (simpler than K8s, popular via Saltbox/CloudBox).

---

### 10. 📋 Debian Package

**Target directory:** `deploy/packages/debian/`

**What it provides:**
- `debian/` directory with `control`, `rules`, `postinst`, `prerm`
- Buildable via `dpkg-buildpackage`
- Installs binary to `/usr/bin/`, config to `/etc/muxcore/`, systemd unit
- `postinst` creates `muxcore` user and enables service

**CI:** Not gating releases. Package repo is separate concern.

**Maintenance:** ⚠️ Best-effort. Requires package repo with GPG signing. Likely to drift without dedicated maintainer.

**When to use:** `apt install muxcored` on Debian/Ubuntu.

---

### 11. 📋 RPM Package

**Target directory:** `deploy/packages/rpm/`

**What it provides:**
- RPM spec file (`muxcored.spec`)
- Buildable via `rpmbuild` or `mock`
- Same structure as .deb for the Red Hat ecosystem

**Maintenance:** ⚠️ Best-effort. Same burden as .deb for a smaller audience.

**When to use:** `dnf install muxcored` on Fedora/RHEL.

---

### 12. 📋 AUR Package

**Target directory:** `deploy/packages/aur/` (reference PKGBUILD)

**What it provides:**
- Reference PKGBUILD that downloads the release tarball, extracts the binary, installs with systemd unit
- Validates with cosign

**Maintenance:** ⚠️ Best-effort. The reference PKGBUILD is a starting point for the community. The actual AUR package is community-maintained.

**When to use:** `yay -S muxcored` on Arch Linux.

---

### 13. 📋 NixOS Module

**Target directory:** `deploy/nix/`

**What it provides:**
- Nix flake with NixOS module:
  ```nix
  {
    services.muxcore = {
      enable = true;
      tag = "default";
      config = ./muxcore.json;
    };
  }
  ```
- Builds from source or pulls prebuilt binary
- Manages systemd, config, TLS certs, user/group
- Optional Prometheus exporter integration

**Maintenance:** ⚠️ Best-effort. Nixpkgs has its own review process. Flake may drift from nixpkgs version.

**When to use:** Declarative, reproducible, rollback-capable deployments on NixOS.

---

### 14. 📋 Homebrew Formula

**Target directory:** `deploy/packages/homebrew/`

**What it provides:**
- Homebrew formula that downloads the Darwin release tarball and installs the binary
- Launchd plist for autostart on macOS

**Usage:**
```bash
brew install muxcore/tap/muxcored
```

**Maintenance:** ⚠️ Best-effort. Tiny production audience (macOS media servers are rare).

**When to use:** Local development or Mac Mini server on macOS.

---

### 15. 📋 Terraform / OpenTofu Module

**Target directory:** `deploy/terraform/`

**What it provides:**
- `modules/muxcore-ecs/` — deploys to AWS ECS (Fargate or EC2) with ALB, CloudWatch logs, EFS
- `modules/muxcore-vm/` — provisions a cloud VM with binary, systemd, security groups
- Root module for VPC, subnets, DNS, TLS cert (ACM)

**Maintenance:** ⚠️ Best-effort. Requires cloud credentials. Overlaps with Helm + Ansible.

**When to use:** Infrastructure-as-code for cloud deployments. Provisioning the backing infra (VPC, firewall, DNS, S3) rather than the app itself.

---

## Quick Reference

| # | Method | Status | Effort | Audience | Type |
|---|--------|--------|--------|----------|------|
| 1 | Binary tarball | ✅ Active | — | All | Package |
| 2 | systemd | ✅ Active | — | Linux | Service mgmt |
| 3 | Docker | ✅ Active | — | Container users | Container |
| 4 | Docker Compose | ✅ Active | — | Container users | Orchestration |
| 5 | SSH deploy | ✅ Active | Hours | Sysadmins | Push |
| 6 | Helm chart | ✅ Active | Days | K8s users | Orchestration |
| 7 | Kustomize | ✅ Active | Hours | K8s users | Orchestration |
| 8 | Ansible role | ✅ Active | Days | Homelab automation | Automation |
| 9 | Nomad job | ⚠️ Best-effort | Hours | Nomad users | Orchestration |
| 10 | .deb package | ⚠️ Best-effort | Days | Debian/Ubuntu | Package |
| 11 | .rpm package | ⚠️ Best-effort | Days | Fedora/RHEL | Package |
| 12 | AUR PKGBUILD | ⚠️ Best-effort | Minutes | Arch Linux | Package |
| 13 | NixOS module | ⚠️ Best-effort | Hours | NixOS | Package |
| 14 | Homebrew | ⚠️ Best-effort | Minutes | macOS | Package |
| 15 | Terraform | ⚠️ Best-effort | Weeks | Cloud | IaC |

---

## Contributing

For 🟡 Active methods: file a bug or PR against the main repo. CI will catch regressions.

For ⚠️ Best-effort methods: these are community-sustained. If something is broken or outdated:
1. Submit a PR to fix it
2. CI runs basic syntax validation on changes
3. Core maintainers will review but won't proactively update

**All deployment methods welcome.** If you maintain a deployment format not listed here, open a PR to add it.
