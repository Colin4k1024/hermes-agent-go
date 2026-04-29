# Delivery Plan: User Memory Persistence (v0.5.0)

## Version Target

v0.5.0 — per-user cross-session memory with rule-based extraction

## Work Breakdown

| Phase | Description | Files |
|-------|-------------|-------|
| 1 | Memory extraction engine (regex-based, EN+ZH) | `memory_extractor.go` |
| 2 | Memory + session context injection into LLM system prompt | `mockchat.go` |
| 3 | Memory management REST API (list/delete memories, session history) | `memory_api.go` |
| 4 | Server wiring (routes, pool passthrough, CORS headers) | `server.go`, `saas.go` |
| 5 | E2E Playwright tests (cross-session recall, tenant isolation) | `playwright_tenant_test.mjs` |

## Key Decisions

- Direct `pgxpool.Pool` access for memory operations (vs Store interface) — memory is a specialized concern not covered by the generic Store
- Rule-based extraction (zero LLM cost) with semantic key derivation
- 50-entry per-user limit with LRU eviction via SQL DELETE subquery
- 4KB cap on cross-session context injection to avoid prompt bloat
