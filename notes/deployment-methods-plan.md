# Deployment Methods Plan

## Existing

| Method | Files | Status |
|--------|-------|--------|
| Bare-metal binary | `.goreleaser.yaml` (linux + darwin, amd64 + arm64) | ✅ Released with every tag |
| systemd | `deploy/systemd/muxcored.service` | ✅ Production-grade, security hardened |
| Docker | `Dockerfile`, `docker-compose.yml` | ✅ Multi-stage, distroless, read-only rootfs |

---

## Short Term (Phase 1) — Owned & Maintained

### 1. Helm Chart

**Directory:** `deploy/helm/muxcore/`

**What:**
- Standard helm chart with `values.yaml`, `Chart.yaml`, templates for:
  - `Deployment` (with health checks, resource limits, security context)
  - `Service` (cluster + optional LoadBalancer/NodePort)
  - `ConfigMap` (muxcore.json injected from values)
  - `Secret` (TLS certs, join tokens)
  - `Ingress` (optional, with TLS)
  - `PersistentVolumeClaim` (audit logs, module cache)
  - `ServiceMonitor` (Prometheus operator integration)
- Support `--set` overrides for most config

**Why:** Industry expectation for containerized services. Enables GitOps, rolling updates, and multi-node clusters.

**Ownership:** Actively maintained, tested in CI with `helm lint` and `helm template` smoke tests. Chart version bumps alongside core releases.

### 2. Kustomize

**Directory:** `deploy/kustomize/` with `base/` and `overlays/`

**What:**
- `deploy/kustomize/base/` — vanilla Deployment, Service, ConfigMap, PVC (same shape as Helm templates but static YAML)
- `deploy/kustomize/overlays/production/` — resource limits, replica counts, prod TLS
- `deploy/kustomize/overlays/development/` — single replica, self-signed certs

**Why:** Lightweight alternative to Helm. No Tiller, no chart museum, pure `kubectl apply -k`. Preferred by users who vendor everything in a monorepo or use ArgoCD with plain YAML.

**Relationship to Helm:** Both are generated from the same underlying template sources. Kustomize overlays are not duplicative — they serve a different UX (static YAML vs parameterized chart). The Helm chart can be used to generate Kustomize bases via `helm template`.

**Ownership:** Actively maintained. CI validates both paths don't diverge.

### 3. Ansible Role

**Directory:** `deploy/ansible/`

**What:**
Installs and configures muxcored on a target host:
- Downloads the binary from GitHub releases (or builds from source)
- Creates `muxcore` user + group
- Deploys systemd unit (the existing `deploy/systemd/muxcored.service`)
- Writes config file from vars
- Sets up TLS certs (self-signed or custom CA)
- Configures logrotate for audit logs
- Starts/enables the service

Optional features:
- `--tag` selection
- Module deployment via spool tags
- Firewall rules (ufw/firewalld)

**Why:** Ansible is the standard for homelab orchestration. One `ansible-playbook` run goes from bare OS to running muxcored. Covers the gap between "I have a binary" and "it's running securely."

**Ownership:** Actively maintained. CI runs `ansible-playbook --syntax-check` on the playbook.

### 4. SSH-based Deploy Script

**Directory:** `deploy/ssh-deploy.sh`

**What:**
A single bash script that:
1. SCPs the locally-built `muxcored` binary (or downloads a release) to a remote host
2. SSHes in to stop the old service, replace the binary, restart
3. Optionally copies a config file, TLS certs, and the cosign public key

Signature: `./deploy/ssh-deploy.sh user@host [--release v1.2.3] [--config muxcore.json]`

**Why:** Zero dependencies — just bash, ssh, and scp. Works on any Linux machine. Useful for quick deploys without Ansible or Docker. The simplest possible "push to prod."

**Ownership:** Actively maintained. Small surface area (single script).

---

## Long Term (Phase 2) — Community Best-Effort ⚠️

> **These deployment methods exist in the repo but are NOT actively maintained by core contributors.**
> They are contributed and updated by the community. They may lag behind the latest release.
> If something is broken, you are encouraged to submit a PR to fix it.
> Core CI will run basic syntax validation on any changes, but will not gate releases on their correctness.

### 5. Nomad Job Spec

**Directory:** `deploy/nomad/`

**What:**
A Nomad job file (`muxcore.nomad`) that runs muxcored as a Docker or `raw_exec` workload:
- `job "muxcore"` with a `group` stanza
- Health check via `service` block + `check` stanza
- Volume mounts for audit logs and module cache
- Resource limits (CPU, memory)
- Optional Consul Connect integration for service mesh

**Why:** Nomad is simpler than K8s and popular in the homelab space (Saltbox, CloudBox use it). Caters to the "one binary, one job file" crowd.

**⚠️ Caveat:** Nomad job DSL changes across versions. The spec targets Nomad 1.7+ but may need adjustments for future releases.

### 6. Debian / Ubuntu Package (.deb)

**Directory:** `deploy/packages/debian/`

**What:**
- `debian/` directory with `control`, `rules`, `postinst`, `prerm`, `service` files
- Buildable via `dpkg-buildpackage` or `debuild`
- Installs `muxcored` to `/usr/bin/`, config to `/etc/muxcore/`, systemd unit to `/lib/systemd/system/`
- `postinst` creates `muxcore` user, sets up directories, enables the service

**Why:** `apt install muxcored` is the lowest-friction install on the most common homelab OS. Handles upgrades, pre-removal stop, post-install start automatically.

**⚠️ Caveat:** Requires a package repo with GPG signing and CI distribution. Only covers Debian derivatives. Binary must be built per-architecture per-release. Likely to drift without dedicated maintainer.

### 7. RPM Package (Fedora / RHEL)

**Directory:** `deploy/packages/rpm/`

**What:**
- RPM spec file in `deploy/packages/rpm/muxcored.spec`
- Buildable via `rpmbuild` or `mock`
- Same installation structure as the .deb

**Why:** Covers the Red Hat ecosystem. Some homelab users run Fedora Server or Rocky Linux.

**⚠️ Caveat:** Same maintenance burden as .deb for a smaller audience. Duplicative in spirit — two package formats to maintain for the same outcome. Likely to drift.

### 8. Arch Linux AUR Package

**Repository:** Ideally a community member maintains `muxcored` in the AUR. The project provides a reference PKGBUILD in `deploy/packages/aur/`.

**What:**
A PKGBUILD that:
- Downloads the release tarball from GitHub
- Extracts the prebuilt binary and installs it with the systemd unit
- Validates with `gpg` or `cosign`

**Why:** Arch is disproportionately popular in the homelab/selfhosted community. AUR makes install/upgrade trivial.

**⚠️ Caveat:** The AUR is inherently community-maintained. The project's reference PKGBUILD is a starting point, not a commitment. Any Arch user can `yay -S muxcored` once it's in the AUR regardless of upstream maintenance. The reference PKGBUILD in the repo is provided as-is, may be out of date, and is not CI-tested.

### 9. NixOS Module

**Directory:** `deploy/nix/`

**What:**
A Nix flake with a NixOS module:
```nix
{
  services.muxcore = {
    enable = true;
    tag = "default";
    config = ./muxcore.json;
    package = pkgs.muxcored;  # from the flake or nixpkgs
  };
}
```
- Builds muxcored from source (via `buildGoModule`) or pulls the prebuilt binary
- Manages systemd service, config file, TLS certs, user/group
- Optionally integrates with NixOS's `services.prometheus.exporters`

**Why:** NixOS is growing fast in the homelab space. Declarative config, pinning, rollbacks, no state drift. Caters to the "my entire server is one config file" philosophy.

**⚠️ Caveat:** Nix learning curve is steep. Audience is niche. Nixpkgs has its own review/merge process. The project flake may drift from nixpkgs version. Best maintained by the NixOS community rather than core contributors.

### 10. Homebrew (macOS)

**Directory:** `deploy/packages/homebrew/`

**What:**
A Homebrew formula (`Formula/muxcored.rb`) that:
- Downloads the Darwin release tarball from GitHub
- Installs the binary, plist (launchd) for autostart

**Why:** MuxCore already builds Darwin binaries. `brew install muxcore/tap/muxcored` is natural for macOS users doing local development or running a Mac Mini server.

**⚠️ Caveat:** Tiny audience for production deployments — almost no one runs a media server on macOS. Useful primarily for development. Likely to drift without a macOS maintainer.

### 11. Terraform / OpenTofu Module

**Directory:** `deploy/terraform/`

**What:**
A Terraform module that provisions infrastructure for muxcored:
- `modules/muxcore-ecs/` — deploys to AWS ECS (Fargate or EC2) with ALB, CloudWatch logs, EFS for storage
- `modules/muxcore-vm/` — provisions a cloud VM with the binary, systemd, security groups
- Root module wiring for VPC, subnets, DNS, TLS cert (ACM)

**Why:** Useful for the multi-node cluster story. Infrastructure-as-code for the backing infra (VPC, firewall rules, S3 buckets, DNS, TLS) rather than application deployment itself.

**⚠️ Caveat:** Terraform manages infrastructure, not application deployment. Requires cloud credentials, state management, and AWS/GCP/Azure expertise. The application deployment concern overlaps with Ansible and Helm. Must be updated whenever the cloud provider changes APIs. Likely to drift significantly if not owned by a dedicated contributor.

---

## Decision Matrix

| Method | Effort to create | Maintenance burden | Audience size | Redundancy risk |
|--------|-----------------|-------------------|---------------|-----------------|
| Helm | Medium | Medium | Large | Low — unique value for K8s users |
| Kustomize | Low | Low | Medium | Low — complements Helm, not replaces |
| Ansible | Medium | Low | Large | Medium — overlaps with systemd+config docs |
| SSH deploy | Very low | Very low | Small | High — trivial to roll yourself |
| Nomad | Low | Low | Medium | Low — unique orchestration platform |
| .deb | Medium | High | Large | Medium — overlaps with Docker |
| .rpm | Medium | High | Small | High — duplicates .deb |
| AUR | Very low | Very low | Medium | Low — community owned |
| NixOS | Low | Low | Small | Low — unique ecosystem |
| Homebrew | Very low | Very low | Tiny | Low — unique platform |
| Terraform | High | High | Small | Medium — overlaps with Helm+Kustomize |

---

## Implementation Order

```
Phase 1:
  [1] SSH deploy script       (hours, immediate value)
  [2] Kustomize                (hours, structural)
  [3] Helm chart               (days, most impact)
  [4] Ansible role             (days, automation multiplier)

Phase 2 (community-gated):
  [5] Nomad job spec           (hours, low effort)
  [6] NixOS flake              (hours, low effort)
  [7] Reference AUR PKGBUILD   (minutes, trivial)
  [8] Homebrew formula         (minutes, trivial)
  [9] .deb package             (days, complex CI pipeline)
  [10] .rpm package            (days, duplicates .deb effort)
  [11] Terraform module        (weeks, significant scope)
```
