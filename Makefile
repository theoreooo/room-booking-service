.PHONY: up seed swagger test loadtest

up:
	docker compose up --build

seed:
	docker compose up -d postgres
	docker compose up migrate
	docker compose run --rm app ./seed

swagger:
	GOCACHE=$(CURDIR)/.gocache GOFLAGS=-buildvcs=false swag init --parseDependency --parseInternal -g main.go -d cmd/app,internal/auth,internal/room,internal/schedule,internal/slot,internal/booking,internal/httputil -o ./docs

test:
	GOCACHE=$(CURDIR)/.gocache go test ./... -coverprofile=coverage.out -covermode=atomic
	GOCACHE=$(CURDIR)/.gocache go tool cover -func=coverage.out

loadtest:
	docker compose up -d --build app
	docker compose --profile tools run --rm k6 run /work/loadtest/loadtest.js
