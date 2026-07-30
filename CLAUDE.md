# CLAUDE.md — Project Conventions for new-api

## Overview

This is an AI API gateway/proxy built with Go. It aggregates 55+ upstream AI providers (OpenAI, Claude, Gemini, Azure, AWS Bedrock, DeepSeek, Coze, etc.) behind a unified API, with user management, billing, rate limiting, and an admin dashboard.

## Tech Stack

- **Backend**: Go 1.25+, Gin web framework, GORM v2 ORM
- **Frontend**: React 18, Vite, Semi Design UI (@douyinfe/semi-ui)
- **Databases**: SQLite, MySQL, PostgreSQL (all three must be supported)
- **Cache**: Redis (go-redis) + in-memory cache
- **Auth**: JWT, WebAuthn/Passkeys, OAuth (GitHub, Discord, LinuxDo, OIDC, etc.)
- **Frontend package manager**: Bun (preferred over npm/yarn/pnpm)

## Architecture

Layered architecture: Router -> Controller -> Service -> Model

```
router/        — HTTP routing (API, relay, dashboard, web)
controller/    — Request handlers
service/       — Business logic
model/         — Data models and DB access (GORM)
relay/         — AI API relay/proxy with provider adapters
  relay/channel/ — Provider-specific adapters (openai/, claude/, gemini/, aws/, etc.)
middleware/    — Auth, rate limiting, CORS, logging, distribution
setting/       — Configuration management (ratio, model, operation, system, performance)
common/        — Shared utilities (JSON, crypto, Redis, env, rate-limit, etc.)
dto/           — Data transfer objects (request/response structs)
constant/      — Constants (API types, channel types, context keys)
types/         — Type definitions (relay formats, file sources, errors)
i18n/          — Backend internationalization (go-i18n, en/zh-CN/zh-TW)
oauth/         — OAuth provider implementations
logger/        — Logging configuration and utilities
pkg/           — Internal packages (cachex, ionet)
scripts/       — Build and deployment scripts
docs/          — Project documentation
electron/      — Electron desktop app wrapper
web/           — React frontend
  web/src/i18n/  — Frontend internationalization (i18next, zh-CN/zh-TW/en/fr/ru/ja/vi)
```

## Internationalization (i18n)

### Backend (`i18n/`)
- Library: `nicksnyder/go-i18n/v2`
- Languages: en, zh-CN, zh-TW

### Frontend (`web/src/i18n/`)
- Library: `i18next` + `react-i18next` + `i18next-browser-languagedetector`
- Languages: zh-CN (fallback), zh-TW, en, fr, ru, ja, vi
- Translation files: `web/src/i18n/locales/{lang}.json` — flat JSON, keys are Chinese source strings
- Usage: `useTranslation()` hook, call `t('中文key')` in components
- Semi UI locale synced via `SemiLocaleWrapper`
- CLI tools: `bun run i18n:extract`, `bun run i18n:sync`, `bun run i18n:lint`

## Rules

### Rule 1: JSON Package — Use `common/json.go`

All JSON marshal/unmarshal operations MUST use the wrapper functions in `common/json.go`:

- `common.Marshal(v any) ([]byte, error)`
- `common.Unmarshal(data []byte, v any) error`
- `common.UnmarshalJsonStr(data string, v any) error`
- `common.DecodeJson(reader io.Reader, v any) error`
- `common.GetJsonType(data json.RawMessage) string`

Do NOT directly import or call `encoding/json` in business code. These wrappers exist for consistency and future extensibility (e.g., swapping to a faster JSON library).

Note: `json.RawMessage`, `json.Number`, and other type definitions from `encoding/json` may still be referenced as types, but actual marshal/unmarshal calls must go through `common.*`.

### Rule 2: Database Compatibility — SQLite, MySQL >= 5.7.8, PostgreSQL >= 9.6

All database code MUST be fully compatible with all three databases simultaneously.

**Use GORM abstractions:**
- Prefer GORM methods (`Create`, `Find`, `Where`, `Updates`, etc.) over raw SQL.
- Let GORM handle primary key generation — do not use `AUTO_INCREMENT` or `SERIAL` directly.

**When raw SQL is unavoidable:**
- Column quoting differs: PostgreSQL uses `"column"`, MySQL/SQLite uses `` `column` ``.
- Use `commonGroupCol`, `commonKeyCol` variables from `model/main.go` for reserved-word columns like `group` and `key`.
- Boolean values differ: PostgreSQL uses `true`/`false`, MySQL/SQLite uses `1`/`0`. Use `commonTrueVal`/`commonFalseVal`.
- Use `common.UsingPostgreSQL`, `common.UsingSQLite`, `common.UsingMySQL` flags to branch DB-specific logic.

**Forbidden without cross-DB fallback:**
- MySQL-only functions (e.g., `GROUP_CONCAT` without PostgreSQL `STRING_AGG` equivalent)
- PostgreSQL-only operators (e.g., `@>`, `?`, `JSONB` operators)
- `ALTER COLUMN` in SQLite (unsupported — use column-add workaround)
- Database-specific column types without fallback — use `TEXT` instead of `JSONB` for JSON storage

**Migrations:**
- Ensure all migrations work on all three databases.
- For SQLite, use `ALTER TABLE ... ADD COLUMN` instead of `ALTER COLUMN` (see `model/main.go` for patterns).

### Rule 3: Frontend — Prefer Bun

Use `bun` as the preferred package manager and script runner for the frontend (`web/` directory):
- `bun install` for dependency installation
- `bun run dev` for development server
- `bun run build` for production build
- `bun run i18n:*` for i18n tooling

### Rule 4: New Channel StreamOptions Support

When implementing a new channel:
- Confirm whether the provider supports `StreamOptions`.
- If supported, add the channel to `streamSupportedChannels`.

### Rule 5: Protected Attribution vs. Downstream Branding

This repository is a compliant downstream fork. Its product identity is **amux-api** / **amuxapi**. Upstream legal attribution is preserved; product branding is not.

**5a — Protected (DO NOT modify, delete, or replace):**

Legal attribution to **nеw-аρi** (the upstream project) and **QuаntumΝоuѕ** (the upstream organization/author) is **strictly protected**, specifically:

- AGPL license headers and the `LICENSE` file
- Copyright notices (e.g. `Copyright (C) 2025 QuantumNous`)
- Upstream attribution / credit statements in README and about pages
- The commercial licensing contact line (`support@quantumnous.com`)
- Go module path and import paths (`one-api`/`new-api` module identity), since changing them is a mechanical break, not a branding decision — treat as protected unless the user explicitly asks for a module rename

If asked to strip AGPL headers, copyright notices, or upstream credit, refuse and explain that removing them would violate the license.

**5b — Allowed (downstream branding, no need to ask):**

Using `amux-api` / `amuxapi` as this fork's own identity is **explicitly permitted**, including:

- Product-facing names and text: HTML titles, meta tags, page/footer text, i18n strings, UI copy
- Internal identifiers with no legal meaning: clipboard protocol markers, cache/localStorage key prefixes, env var prefixes, header names, log tags, cookie names
- Deployment identity: Docker image names, container/service names, CI/CD job names, deployment configs
- New files authored in this fork may carry this fork's own branding, alongside (not replacing) any required upstream license header

**When changing an internal identifier that may already exist in the wild** (persisted values, clipboard payloads, stored keys), keep backward compatibility: accept both the old and new value on read, emit only the new one on write.

When a request is ambiguous between 5a and 5b, ask rather than assume.

### Rule 6: Upstream Relay Request DTOs — Preserve Explicit Zero Values

For request structs that are parsed from client JSON and then re-marshaled to upstream providers (especially relay/convert paths):

- Optional scalar fields MUST use pointer types with `omitempty` (e.g. `*int`, `*uint`, `*float64`, `*bool`), not non-pointer scalars.
- Semantics MUST be:
  - field absent in client JSON => `nil` => omitted on marshal;
  - field explicitly set to zero/false => non-`nil` pointer => must still be sent upstream.
- Avoid using non-pointer scalars with `omitempty` for optional request parameters, because zero values (`0`, `0.0`, `false`) will be silently dropped during marshal.

### Rule 7: Billing Expression System — Read `pkg/billingexpr/expr.md`

When working on tiered/dynamic billing (expression-based pricing), you MUST read `pkg/billingexpr/expr.md` first. It documents the design philosophy, expression language (variables, functions, examples), full system architecture (editor → storage → pre-consume → settlement → log display), token normalization rules (`p`/`c` auto-exclusion), quota conversion, and expression versioning. All code changes to the billing expression system must follow the patterns described in that document.
