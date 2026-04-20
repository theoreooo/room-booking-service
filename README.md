# Room Booking Service

Standalone Go backend for managing meeting rooms, schedules, available time slots, and bookings.

The repository keeps the full implementation history from the first prototype and continues as an independent pet project focused on backend design, API ergonomics, and reproducible local development.

## Highlights

- room, schedule, slot, and booking management
- JWT-based auth with role separation for `admin` and `user`
- email/password registration and login
- optional conference link creation during booking through a mock integration
- idempotent booking cancellation
- Swagger generation from handler annotations
- unit and integration-style HTTP tests
- Docker Compose local environment
- reproducible k6 load test for the hottest endpoint

## Stack

- Go
- Chi
- PostgreSQL
- Docker Compose
- Swagger
- k6

## Why This Project Is Interesting

- Slots are pre-generated for a rolling window of upcoming days instead of being computed on every hot-path request.
- Availability reads are backed by persisted slot records and database indexes.
- Booking creation handles external conference-link side effects with best-effort cleanup on partial failures.
- Cancellation is implemented as an idempotent operation, which keeps client behavior simple and robust.

## Quick Start

Start the full local stack:

```bash
make up
```

or

```bash
docker compose up --build
```

The API becomes available at `http://localhost:8080`.

An example environment file is available at [`.env.example`](./.env.example), but the Compose defaults are enough for a zero-config local start.

Seed demo data:

```bash
make seed
```

Health check:

```bash
curl -i http://localhost:8080/_info
```

Swagger UI:

```text
http://localhost:8080/swagger/index.html
```

## Demo Users

After `make seed`:

- `admin@example.com` / `Admin123!`
- `user@example.com` / `User123!`

## Useful Commands

Run tests:

```bash
make test
```

Regenerate Swagger:

```bash
make swagger
```

Run the load test:

```bash
make loadtest
```

## Booking With Conference Links

`POST /bookings/create` supports the optional flag:

```json
{
  "slotId": "uuid",
  "createConferenceLink": true
}
```

The current implementation uses a mock conference provider and models several failure scenarios:

- skip external calls when `createConferenceLink=false`
- reject booking creation with `503` when the mock provider is unavailable
- attempt best-effort cleanup when the external link was created but local persistence fails

Failure simulation flags:

- `CONFERENCE_MOCK_FAIL_CREATE=true`
- `CONFERENCE_MOCK_FAIL_DELETE=true`

## Quality Notes

- `go test ./...` runs the project test suite
- local coverage report is written to `coverage.out`
- the repository includes a GitHub Actions CI workflow for build and test checks

## Repository Layout

- [`cmd/app`](./cmd/app) application entrypoint and router
- [`cmd/seed`](./cmd/seed) demo data seeding command
- [`internal`](./internal) domain, services, repositories, handlers, middleware, workers
- [`migrations`](./migrations) database schema migrations
- [`docs`](./docs) generated Swagger artifacts
- [`loadtest`](./loadtest) k6 scenario and sample results
