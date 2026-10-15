# Gophermart — Loyalty Rewards Service

An educational Go backend for the Gophermart loyalty program. The service is designed to accept order numbers, retrieve reward calculations from an external service, and let users track and redeem loyalty points.

**Status: in development.** User registration, login, authenticated order submission, order listing, balance retrieval, and point redemption with PostgreSQL storage are implemented. See the [project specification](SPECIFICATION.md) (in Russian) for the full requirements.

## Implemented Features

- `POST /api/user/register` endpoint with JSON, login, and password validation.
- `POST /api/user/login` endpoint with password verification and JWT issuance.
- `POST /api/user/orders` endpoint with cookie authentication, Luhn validation, and duplicate ownership checks.
- `GET /api/user/orders` endpoint returning the authenticated user's orders, newest first, with statuses and available accruals.
- `POST /api/user/balance/withdraw` endpoint with authenticated, transactional point redemption.
- Password hashing with bcrypt.
- User storage in PostgreSQL with unique logins.
- JWT issuance using HS256, with a three-hour token lifetime, delivered in the HttpOnly `user` cookie.
- JWT creation and validation functions.
- Configurable logging with zap and graceful HTTP server shutdown on Ctrl+C.
- Tests for registration, login, order submission, routing, JWT handling, and repository behavior.

`GET /api/user/balance` returns the available balance and total withdrawals from stored operations. Reward processing and integration with the rewards calculation service are not yet implemented.

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
psql -h localhost -p 5432 -U postgres -d gophermart -v ON_ERROR_STOP=1 -f internal/migrations/000002_order_accrual.up.sql
psql -h localhost -p 5432 -U postgres -d gophermart -v ON_ERROR_STOP=1 -f internal/migrations/000003_withdrawals.up.sql
```

If the database already exists, skip the database creation command. The migrations create the `users`, `orders`, and `withdrawals` tables, add the optional `accrual` column, and index orders and withdrawals by user and time. The application applies all three idempotent migrations automatically at startup, including on an existing database. Apply down migrations in reverse order.

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

### POST /api/user/login

Header: `Content-Type: application/json`. Send the same JSON fields as registration (`login` and `password`). The handler accepts a single JSON object up to 1 MiB; login must not be blank and password must not be empty.

```powershell
$body = @{ login = 'demo'; password = 'demo-password' } | ConvertTo-Json
Invoke-WebRequest -Uri 'http://localhost:8080/api/user/login' -Method Post -ContentType 'application/json' -Body $body -SessionVariable session
```

| HTTP Status | Meaning |
| --- | --- |
| `200 OK` | Authenticated; JWT issued in the HttpOnly `user` cookie |
| `400 Bad Request` | Malformed JSON or invalid login data |
| `401 Unauthorized` | Unknown login or incorrect password |
| `415 Unsupported Media Type` | Unsupported request content type |
| `500 Internal Server Error` | Internal authentication error |

### POST /api/user/orders

Requires the `user` cookie issued by registration or login. Header: `Content-Type: text/plain` (an optional charset parameter is accepted). The body is the order number itself, without JSON quotes, spaces, or a trailing newline:

```text
12345678903
```

The number must contain only ASCII digits and pass the Luhn checksum: moving right to left, double every second digit, subtract 9 from doubled values greater than 9, and require the total to be divisible by 10. Validation works directly with digits, preserving leading zeroes and avoiding integer overflow for long numbers.

Using the session from registration or login:

```powershell
Invoke-WebRequest -Uri 'http://localhost:8080/api/user/orders' -Method Post -ContentType 'text/plain' -Body '12345678903' -WebSession $session
```

New orders are stored with their owner, status `NEW`, and upload timestamp. Order numbers are globally unique; concurrent submissions cannot overwrite the original owner. Repeating a submission does not change its status or timestamp. Reward calculation is not implemented yet.

| HTTP Status | Meaning |
| --- | --- |
| `200 OK` | Order already submitted by this user |
| `202 Accepted` | New order stored for processing |
| `400 Bad Request` | Missing/invalid content type, empty body, or body read error |
| `401 Unauthorized` | Missing, invalid, or expired authentication cookie |
| `409 Conflict` | Order already belongs to another user |
| `422 Unprocessable Entity` | Non-digit characters or invalid Luhn checksum |
| `500 Internal Server Error` | Internal storage error |

### GET /api/user/orders

Requires the `user` cookie issued by registration or login. No request body or `Content-Type` header is needed. Only the authenticated user's orders are returned, sorted by upload time from newest to oldest (ties are ordered by number descending).

```powershell
Invoke-WebRequest -Uri 'http://localhost:8080/api/user/orders' -Method Get -WebSession $session
```

A successful response has `Content-Type: application/json`:

```json
[
  {
    "number": "9278923470",
    "status": "PROCESSED",
    "accrual": 500.5,
    "uploaded_at": "2026-10-13T12:00:00Z"
  },
  {
    "number": "12345678903",
    "status": "NEW",
    "uploaded_at": "2026-10-13T11:00:00Z"
  }
]
```

Order numbers remain strings, including leading zeroes. `uploaded_at` uses RFC3339. Statuses are `NEW`, `PROCESSING`, `INVALID`, or `PROCESSED`. `accrual` is omitted when no amount is stored; a stored zero is returned as `0`. The response reflects stored data; new submissions remain `NEW` until reward processing is implemented.

| HTTP Status | Meaning |
| --- | --- |
| `200 OK` | Orders returned as a JSON array |
| `204 No Content` | User has no orders; response body is empty |
| `401 Unauthorized` | Missing, invalid, or expired authentication cookie |
| `500 Internal Server Error` | Storage or response serialization error |

### GET /api/user/balance

Requires the `user` cookie issued by registration or login. No request body or `Content-Type` header is needed.

```powershell
Invoke-WebRequest -Uri 'http://localhost:8080/api/user/balance' -Method Get -WebSession $session
```

Example response (`Content-Type: application/json`):

```json
{
  "current": 500.5,
  "withdrawn": 42
}
```

`current` is the sum of accruals on this user's `PROCESSED` orders minus all their withdrawals. `withdrawn` is the total amount withdrawn over the lifetime of the account. Both totals are read in a single database statement; operations belonging to other users are excluded. PostgreSQL performs the sums and subtraction using `NUMERIC` before the result is converted for the JSON response.

Both fields are always present. A user without operations receives `200 OK` with `{"current":0,"withdrawn":0}`. New orders do not contribute to the balance. Automatic reward processing is not yet implemented.

| HTTP Status | Meaning |
| --- | --- |
| `200 OK` | Current balance and total withdrawals returned |
| `401 Unauthorized` | Missing, invalid, or expired authentication cookie |
| `500 Internal Server Error` | Storage or response serialization error |

### Redeem Points

`POST /api/user/balance/withdraw` requires the authenticated `user` cookie and `Content-Type: application/json`.

```json
{"order":"2377225624","sum":751}
```

`order` must pass the Luhn check. It identifies a hypothetical new purchase and does not need to exist in `orders`. `sum` must be a positive JSON number. A successful response has an empty body.

The repository locks the user's row within a READ COMMITTED transaction before reading withdrawals and checking funds. Concurrent withdrawals for the same user are serialized. PostgreSQL compares and stores the original decimal amount using `NUMERIC`; no floating-point balance arithmetic is used. The operation records the withdrawal and its timestamp, which are reflected by `GET /api/user/balance`.

An identical retry (same order, user, and numeric amount) returns `200` without another deduction, even if the remaining balance is insufficient. An existing withdrawal with different details returns `409`; this duplicate policy extends the specification.

| HTTP Status | Meaning |
| --- | --- |
| `200 OK` | Withdrawal recorded, or identical request already processed |
| `400 Bad Request` | Invalid JSON, content type, or nonpositive/invalid sum |
| `401 Unauthorized` | Missing, invalid, or expired authentication cookie |
| `402 Payment Required` | Insufficient available points |
| `409 Conflict` | Order already used for a withdrawal with different details |
| `422 Unprocessable Entity` | Invalid order number |
| `500 Internal Server Error` | Database or transaction error |

## Testing and Static Analysis

```powershell
go test ./...
go vet ./...
```

The current tests do not require a running PostgreSQL instance. They cover registration and login, JWT handling, all documented order submission and listing status codes, Luhn validation, long order numbers, request context propagation, and routing. Listing tests check expired cookies, JSON fields, RFC3339 timestamps, absent versus zero accruals, and empty responses. Repository tests use a substitute SQL driver to check insertion, duplicate ownership, user filtering and sort clauses, nullable accruals, and query/scan/iteration failures. They do not verify real PostgreSQL integration or concurrent database behavior.

Balance tests are grouped in `balance_test.go` within the handler and repository packages. They cover decimal and zero totals, a fully spent balance, authentication failures, request context propagation, storage and serialization errors, and SQL clauses for filtering and aggregation. Routing is checked in `server_test.go`. SQL-driver tests do not execute the aggregation against a real PostgreSQL instance.

Withdrawal tests are included in the same balance test files. They cover request validation, insufficient funds, duplicate handling, transaction commit and rollback, and database errors at each operation. These tests do not verify locking or concurrent execution on a live PostgreSQL instance.

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
