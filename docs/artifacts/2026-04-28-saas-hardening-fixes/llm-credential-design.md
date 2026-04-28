# Design: Per-Tenant LLM Credential Isolation

## Status: Design Only — Not Implemented

## Problem

All tenants share a single LLM API key from global config. No per-tenant billing, quota, or revocation.

## Schema Change

```sql
ALTER TABLE tenants ADD COLUMN llm_api_key_encrypted BYTEA;
ALTER TABLE tenants ADD COLUMN llm_provider TEXT;
ALTER TABLE tenants ADD COLUMN llm_model TEXT;
```

- `llm_api_key_encrypted`: AES-256-GCM encrypted with server-side key from `HERMES_ENCRYPTION_KEY` env var.
- Null = use global config fallback.

## Encryption Approach

- Server-side encryption: AES-256-GCM with 12-byte random nonce, stored as `nonce || ciphertext`.
- Key derivation: raw 32-byte key from `HERMES_ENCRYPTION_KEY` env var (hex-encoded).
- Alternative considered: pgcrypto — rejected because key management is harder and couples to PG.

## Agent Factory Change

```go
func (f *AgentFactory) createClient(ctx context.Context, tenantID string) (*llm.Client, error) {
    tenant, err := f.store.Tenants().Get(ctx, tenantID)
    if err != nil {
        return nil, err
    }
    if tenant.LLMAPIKeyEncrypted != nil {
        apiKey := decrypt(tenant.LLMAPIKeyEncrypted)
        provider := tenant.LLMProvider
        model := tenant.LLMModel
        return llm.NewClientWithParams(model, "", apiKey, provider)
    }
    // Fallback to global config
    return llm.NewClient(config.Load())
}
```

## Migration Path

- Backward compatible: existing tenants have NULL encrypted key → global fallback.
- Admin API endpoint for key upload: `PUT /v1/tenants/{id}/llm-credentials`.
- Key rotation: update encrypted column, no downtime.

## Deferred Items

- Billing integration with external system.
- Per-tenant usage quota enforcement.
- Key rotation automation.
- Audit logging of credential access.
