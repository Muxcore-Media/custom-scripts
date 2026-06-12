# Wiki Documentation Changes Pending (from certauth implementation)

The following wiki pages need to be updated to reflect the new certificate authority and mTLS module registration system:

## New Pages Required

1. **Module TLS Authentication** — Document the full architecture:
   - Core as internal CA (auto-generates on first start)
   - Core-spawned modules get auto-issued certs
   - External modules use one-time bootstrap tokens
   - Configuration: `ca_cert_dir`, `mtls_enabled`, `ca_cert_file`

2. **gRPC Services — BootstrapRegister** — Document the new RPC:
   - `module.v1.ModuleRegistration/BootstrapRegister`
   - Request: token (one-time), module_id
   - Response: signed_cert, key_pem, ca_cert

## Existing Pages Requiring Updates

3. **Deployment Guide** — Update TLS section:
   - Add `MUXCORE_MTLS_ENABLED=true` flag for production
   - Default CA path: `~/.muxcore/ca/`
   - Security: CA private key at `ca.key` (0600 permissions)

4. **Module Developer Guide** — Update registration flow:
   - Sidecar modules spawned by core get `--muxcore-tls-cert` and `--muxcore-tls-key` flags automatically
   - External modules use BootstrapRegister RPC with a one-time token

5. **Security Model** — Add section on cryptographic module identity:
   - TLS client certificate CN = module ID (verified at Register)
   - No more x-caller-id trust (peer address binding is dev-only fallback)
