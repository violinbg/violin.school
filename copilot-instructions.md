# Copilot Instructions — Violin School

## Project Overview
**Violin School** is an online learning platform. This is the self-hosted web application powering it.
It is **not** related to music lessons — "Violin" is the brand family name.

## Tech Stack

| Layer | Technology |
|-------|------------|
| Backend | Go 1.25 + Gin web framework |
| Database | SQLite (modernc — pure Go, no CGO) with WAL mode |
| Frontend | Angular 21 (standalone components) |
| UI Components | PrimeNG 21 + PrimeIcons |
| i18n | @ngx-translate/core (6 languages: en, bg, es, ja, ko, zh) |
| Auth | JWT (access token) + refresh token (stored in SQLite) |
| Deployment | Single Go binary with embedded Angular SPA + Dockerfile |

## Project Structure

```
violin.school/
├── main.go                    # Entry point; embeds UI + migrations
├── go.mod                     # Module: github.com/violinbg/violin.school
├── package.json               # Root npm: dev script (concurrently) + build scripts
├── Dockerfile                 # Multi-stage: Node build → Go build → Alpine runtime
├── copilot-instructions.md    # This file
├── README.md
│
├── internal/
│   ├── server/                # Gin HTTP server + API route handlers
│   │   ├── server.go          # Engine setup, SPA serving, cache headers
│   │   ├── routes.go          # Route registration (calls all register* functions)
│   │   ├── middleware.go      # AuthRequired, AdminRequired middleware
│   │   ├── routes_health.go   # GET /api/v1/health
│   │   ├── routes_setup.go    # POST /api/v1/setup (first-time init)
│   │   ├── routes_auth.go     # /api/v1/auth/* (login, register, refresh, logout, captcha)
│   │   ├── routes_courses.go  # GET /api/v1/courses (protected)
│   │   ├── routes_users.go    # /api/v1/users (admin-only CRUD)
│   │   └── routes_admin.go    # /api/v1/admin/settings
│   ├── database/              # SQLite connection + migration runner
│   ├── models/                # Shared data structures (User, AppConfig, Course)
│   ├── auth/                  # JWT generation/validation + password hashing
│   ├── captcha/               # In-memory captcha store
│   └── ratelimit/             # Rate limiting middleware
│
├── migrations/                # SQL migration files (001–010, run in order on startup)
│
└── ui/                        # Angular 21 application
    ├── src/
    │   ├── app/
    │   │   ├── app.ts         # Root component (selector: vs-root)
    │   │   ├── app.routes.ts  # Client-side routes
    │   │   ├── app.config.ts  # Angular providers
    │   │   ├── home/          # Public landing page
    │   │   ├── auth/          # Login + register dialogs
    │   │   ├── setup/         # First-time setup page
    │   │   ├── dashboard/     # Authenticated user home
    │   │   ├── users/         # Admin user management
    │   │   ├── core/
    │   │   │   ├── services/  # Auth, User, Admin, Language services
    │   │   │   ├── guards/    # Route guards (auth, admin, setup, initialized)
    │   │   │   └── interceptors/ # JWT auth interceptor
    │   │   └── shared/
    │   │       └── components/app-header/  # Reusable page header
    │   └── assets/i18n/       # Translation files (en.json is source of truth)
    └── dist/browser/          # Built output (embedded in Go binary via go:embed)
```

## Naming Conventions

### Angular Components
- **Selector prefix**: `vs-` (Violin School)
  - e.g. `vs-root`, `vs-home`, `vs-dashboard`, `vs-app-header`
- **Files**: `kebab-case.component.ts / .html / .scss`
- **New features** go in a dedicated folder under `ui/src/app/`

### Go
- **Module path**: `github.com/violinbg/violin.school`
- **Package names**: match directory names (e.g., `package server`, `package models`)
- **Route files**: `routes_<feature>.go` — each exports a single `register<Feature>Routes` function
- **Handler functions**: `handle<Verb><Entity>` (e.g., `handleListCourses`)

### Database
- **DB filename**: `violin.school.db` (default, overridable via `DB_PATH` env var)
- **Migrations**: sequential SQL files `NNN_description.sql` in `migrations/`
- Each migration is run once (tracked in `schema_migrations` table)

### Database Audit Rule (Domain Tables)
- For new **domain/product** tables, always include:
  - `created_by` (TEXT)
  - `created_at` (DATETIME)
  - `updated_by` (TEXT)
  - `updated_at` (DATETIME)
- Add `active` only when the entity needs soft-disable behavior (do not force it on append-only/event tables).
- Internal/framework tables (e.g. migration tracking internals) are exempt.

### localStorage Keys
- `vs_token` — JWT access token
- `vs_refresh_token` — Refresh token

## API Design Patterns

- **Base path**: `/api/v1`
- **Auth**: JWT Bearer token via `Authorization` header (injected by `auth.interceptor.ts`)
- **Public routes**: health, setup, auth (login/register/captcha/refresh/logout)
- **Protected routes**: require valid JWT (enforced by `AuthRequired` middleware)
- **Admin routes**: require JWT + `admin` role (enforced by `AdminRequired` middleware)
- **Error responses**: `{"error": "message"}` with appropriate HTTP status codes
- **Success responses**: JSON object or array; `201 Created` for new resources

## Angular Patterns

- **Standalone components** — no NgModules, all imports declared per component
- **Signals** for reactive state (`signal()`, `computed()`)
- **`inject()`** instead of constructor injection
- **`firstValueFrom()`** for HTTP observables in async methods
- **PrimeNG** for all UI components; use `styleClass` for custom classes
- **TranslatePipe** (`| translate`) + `TranslateService.instant()` for all user-facing strings
- All new i18n keys go in `ui/src/assets/i18n/en.json` first, then other language files

## Adding a New Feature

1. **Backend**: Create `internal/server/routes_<feature>.go`, add `register<Feature>Routes` call in `routes.go`
2. **Migration**: Add `migrations/NNN_<feature>.sql` if new tables are needed
3. **Model**: Add struct to `internal/models/models.go` if needed
4. **Frontend component**: Create `ui/src/app/<feature>/<feature>.component.ts|html|scss` with `vs-<feature>` selector
5. **Route**: Add to `ui/src/app/app.routes.ts`
6. **i18n**: Add a `FEATURE_NAME` key block to all `assets/i18n/*.json` files (en.json first)
7. **Navigation**: Add link in `dashboard.component.html` and/or `app-header`

## Development

```bash
# Install dependencies
npm install && npm install --prefix ui

# Start dev server (Go API + Angular dev server with proxy)
npm run dev

# Build production binary
npm run build
```

- Go API runs on `:8080`
- Angular dev server proxies `/api/*` to `:8080` (see `proxy.conf.json`)
- Production: single binary serves both API and SPA

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DB_PATH` | `violin.school.db` | SQLite database file path |
