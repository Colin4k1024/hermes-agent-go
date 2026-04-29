# PRD: User Memory Persistence Across Sessions (v0.5.0)

## Background

Hermes SaaS multi-tenant platform stores user conversation context only within a single session (mockChatStore keyed by tenantID:sessionID). When users switch sessions, all identity, preferences, and conversation history is lost. This degrades the user experience for returning users who expect the AI to remember prior interactions.

## Goals & Success Criteria

- Users can switch sessions and the AI remembers their identity, preferences, and key facts
- Memory is isolated by tenant + user (no cross-tenant or cross-user leakage)
- Zero additional LLM cost — rule-based memory extraction only
- Memory limit enforcement (50 entries per user) with LRU eviction
- REST API for memory management (list, delete)
- Session history API (list sessions, view messages)

## User Stories

1. As a user, I can tell the AI "remember: my favorite fruit is mango" and it persists across sessions
2. As a user in a new session, the AI recalls my previously stated preferences
3. As a tenant admin, I can verify that memories are isolated per-user within my tenant
4. As a different tenant's user, I cannot access another tenant's user memories

## Scope

**In Scope**: Memory extraction, PG persistence, cross-session injection, memory/session REST API, E2E tests
**Out of Scope**: LLM-based summarization, vector similarity search, memory sharing between users

## Risk & Dependencies

- Depends on existing `memories` table (migration 22) and `PGMemoryProvider`
- Rule-based extraction may miss complex statements — acceptable for v0.5.0
- Cross-session context injection adds ~4KB to system prompt
