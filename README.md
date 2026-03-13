# violin.school

An online learning platform — self-hosted, privacy-friendly, and easy to deploy.

## Tech Stack

- **Backend:** Go + Gin + SQLite (modernc, pure Go)
- **Frontend:** Angular 21 + PrimeNG + SCSS
- **Deployment:** Single binary with embedded UI

## Quick Start

```bash
# Install root dev tools (first time only)
npm install

# Build everything
npm run build

# Run
./violin.school.exe

# Open http://localhost:8080
```

## Development

```bash
# Install all dependencies (first time only)
npm install && npm install --prefix ui

# Run both backend and Angular dev server concurrently
npm run dev

# Or separately:
npm run dev:server   # Go backend on :8080
npm run dev:ui       # Angular dev server on :4200 (proxies /api to :8080)
```

## First Run

On first launch, navigate to `http://localhost:8080/setup` to create the admin account.

## Deployment

### Docker (Alpine)

Build the container image:

```bash
docker build -t violin-school:latest .
```

Run with a named Docker volume:

```bash
docker run --rm \
-p 8080:8080 \
-e PORT=8080 \
-e DB_PATH=/data/violin.school.db \
-v violin_school_data:/data \
violin-school:latest
```

Run with a host data folder so the SQLite file is created on your machine.

Linux/macOS:

```bash
mkdir -p ./data
ls -lnd data
echo "Look at the above line and enter the uid of the folder? (1000,1001...):"
read uid
docker run -it \
--memory="256m" --memory-reservation="128m" --name violin-school \
--user $uid:$uid \
-p 8080:8080 \
-e PORT=8080 \
-e DB_PATH=/data/violin.school.db \
-v "$(pwd)/data:/data" \
-d violin-school:latest
docker update --restart unless-stopped violin-school
```

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force .\data | Out-Null
docker run --rm `
-p 8080:8080 `
-e PORT=8080 `
-e DB_PATH=/data/violin.school.db `
-v "${PWD}\data:/data" `
violin-school:latest
```

### Runtime Configuration

Configuration resolution order:

1. CLI flags
2. Environment variables
3. Defaults

| Setting | Flag | Env Var | Default |
|---------|------|---------|---------|
| Listen address | `-addr` | `PORT` | `:8080` |
| Database path | `-db` | `DB_PATH` | `violin.school.db` |
| xAI token encryption key (32-byte, base64 or hex) | — | `XAI_TOKEN_ENCRYPTION_KEY` | *(unset)* |

Examples:

```bash
# Env vars only
PORT=9090 DB_PATH=/data/custom.db ./violin.school

# Flags override env vars
PORT=9090 DB_PATH=/data/custom.db ./violin.school -addr :7070 -db ./local.db
```

### xAI Token Encryption Key

When using AI settings/token storage, set `XAI_TOKEN_ENCRYPTION_KEY` so the xAI API token is encrypted at rest.

Requirements:

- Exactly **32 bytes** key material
- Provided as **base64** or **hex**

Generate a base64 key (Linux/macOS):

```bash
openssl rand -base64 32
```

Generate a base64 key (PowerShell):

```powershell
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Minimum 0 -Maximum 256 }))
```

Set it before starting the server:

Linux/macOS:

```bash
export XAI_TOKEN_ENCRYPTION_KEY="<your-generated-key>"
./violin.school
```

PowerShell:

```powershell
$env:XAI_TOKEN_ENCRYPTION_KEY = "<your-generated-key>"
.\violin.school.exe
```

## Project Structure

```
main.go                 Entry point — embeds UI + migrations
internal/server/        Gin HTTP server and API route handlers
internal/database/      SQLite connection and migration runner
internal/models/        Shared data structures
migrations/             SQL migration files (run in order on startup)
ui/                     Angular 21 application
copilot-instructions.md Architecture reference and development conventions
```
