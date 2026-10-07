# Gophermart — Loyalty Rewards Service

An educational Go backend for the Gophermart loyalty program. The service is designed to accept order numbers, retrieve reward calculations from an external service, and let users track and redeem loyalty points.

**Status: in development.** User registration with PostgreSQL storage and JWT issuance is currently implemented. See the [project specification](SPECIFICATION.md) (in Russian) for the full requirements.

## Implemented Features

- `POST /api/user/register` endpoint with JSON, login, and password validation.
- Password hashing with bcrypt.
- User storage in PostgreSQL with unique logins.
- JWT issuance using HS256, with a three-hour token lifetime, delivered in the HttpOnly `user` cookie.
- JWT creation and validation functions.
- Configurable logging with zap and graceful HTTP server shutdown on Ctrl+C.
- Unit tests for registration, JWT handling, and repository input validation.

Login, authorization for protected routes, order processing, integration with the rewards calculation service, balance tracking, and point redemption are not yet implemented.

## Tech Stack

| Component | Technology |
| --- | --- |
| Language | Go; version declared in `go.mod`: `1.26.5` |
| HTTP | `net/http`, chi |
| Database | PostgreSQL, `database/sql`, pgx driver |
| Passwords and tokens | bcrypt, golang-jwt/jwt |
| Configuration and logging | Environment variables, caarlos0/env, zap |

## Running Locally

You will need a Go version compatible with `go.mod`, a running PostgreSQL instance, the `psql` client, and Git. The commands below use PowerShell.

### 1. Clone the Repository

```powershell
git clone https://github.com/selis18/gomart_dipl.git
cd gomart_dipl
go mod download
```

Run the remaining commands from the repository root.

### 2. Prepare the Database

Create the `gophermart` database and apply the SQL migration. Adjust the PostgreSQL host, port, and username for your environment if needed.

```powershell
psql -h localhost -p 5432 -U postgres -c "CREATE DATABASE gophermart;"
psql -h localhost -p 5432 -U postgres -d gophermart -v ON_ERROR_STOP=1 -f internal/migrations/000001_create_tables.up.sql
```

If the database already exists, run only the second command. The migration creates the `users` table. Migrations are not applied automatically at application startup.

### 3. Configure the Environment and Start the Server

Replace `YOUR_PASSWORD` with your local database password. Special characters in the URI username and password must be URL-encoded.

```powershell
$env:DATABASE_URI = 'postgres://postgres:YOUR_PASSWORD@localhost:5432/gophermart?sslmode=disable'
$secretBytes = New-Object byte[] 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try { $rng.GetBytes($secretBytes) } finally { $rng.Dispose() }
$env:SECRET_KEY = [Convert]::ToBase64String($secretBytes)
$env:LOG_LEVEL = 'info'

go run ./cmd/gophermart
```

The server listens at `http://localhost:8080`. Press Ctrl+C to stop it. The connection example uses `sslmode=disable` for a local database. Generating a new `SECRET_KEY` invalidates previously issued tokens.

### Configuration

| Variable | Required | Description |
| --- | --- | --- |
| `DATABASE_URI` | Yes | PostgreSQL connection string |
| `SECRET_KEY` | Yes | JWT signing key |
| `LOG_LEVEL` | No | Log level, such as `debug`, `info`, `warn`, or `error`; defaults to `info` |

The listen address, `localhost:8080`, is currently set in code. Command-line flags and the `RUN_ADDRESS` and `ACCRUAL_SYSTEM_ADDRESS` variables described in the specification are not yet supported. A `.env` file is not loaded automatically.

## Example Request

With the server running, open another PowerShell window and register a user:

```powershell
$body = @{ login = 'demo'; password = 'demo-password' } | ConvertTo-Json
$response = Invoke-WebRequest -Uri 'http://localhost:8080/api/user/register' -Method Post -ContentType 'application/json' -Body $body -SessionVariable session
$response.StatusCode
$session.Cookies.GetCookies([Uri]'http://localhost:8080')
```

A successful response returns `200 OK` with an empty body and a `Set-Cookie` header containing the JWT in the `user` cookie. Registering the same login again returns `409 Conflict`.

### POST /api/user/register

Header: `Content-Type: application/json`.

```json
{
  "login": "demo",
  "password": "demo-password"
}
```

The login must not be empty or contain only whitespace. The password must be between 1 and 72 bytes long. The handler limits the request body to 1 MiB and accepts a single JSON object.

| HTTP Status | Meaning |
| --- | --- |
| `200 OK` | User created; JWT issued in a cookie |
| `400 Bad Request` | Malformed JSON or invalid registration data |
| `409 Conflict` | Login already taken |
| `415 Unsupported Media Type` | Unsupported request content type |
| `500 Internal Server Error` | Internal registration error |

## Testing and Static Analysis

```powershell
go test ./...
go vet ./...
```

The current unit tests do not require a running PostgreSQL instance. They cover successful registration, invalid requests, storage errors, JWT creation and validation, and rejection of empty credentials by the repository. They do not cover integration with a real database.

## Project Structure

```text
cmd/gophermart/       Application entry point, database connection, HTTP server
internal/
  auth/              JWT and cookie helpers
  config/            Environment-based configuration
  handler/           HTTP handlers and their tests
  logger/            Logging setup
  migrations/        SQL migrations
  model/             Request models
  repository/        PostgreSQL data access
SPECIFICATION.md     Full project specification (in Russian)
```

Developed as a diploma project for the Yandex Practicum Go Developer course, based on the course's diploma project template.
