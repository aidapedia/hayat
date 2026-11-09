.PHONY: up dev seed docker db db-down db-reset console test generate lint build images

COMPOSE = docker compose -f deploy/compose/docker-compose.yml
# Local runs use the production binary and commands; only these env vars differ.
LOCAL   = HAYAT_DATABASE_URL='postgres://hayat:hayat@127.0.0.1:55432/hayat?sslmode=disable' \
          HAYAT_MASTER_KEY="$$(cat .dev/master.key)" HAYAT_DEV=true

# Built rather than `go run`, and vite started directly rather than through
# pnpm, so the signals below reach the real processes.
up: db          ## the whole app in one terminal: backend + console; Ctrl+C stops both
	@[ -d console/node_modules ] || (cd console && pnpm install)
	@go build -o .dev/bin/hayat ./cmd/hayat
	@$(LOCAL) ./.dev/bin/hayat migrate
	@$(LOCAL) ./.dev/bin/hayat seed
	@trap 'kill $$api $$web 2>/dev/null; wait' INT TERM EXIT; \
	$(LOCAL) ./.dev/bin/hayat serve & api=$$!; \
	(cd console && exec ./node_modules/.bin/vite) & web=$$!; \
	while kill -0 $$api 2>/dev/null && kill -0 $$web 2>/dev/null; do sleep 1; done

dev: db         ## the backend only: migrate, then serve every role
	@go build -o .dev/bin/hayat ./cmd/hayat
	@$(LOCAL) ./.dev/bin/hayat migrate
	@$(LOCAL) ./.dev/bin/hayat seed
	@$(LOCAL) ./.dev/bin/hayat serve

seed: db        ## dev accounts + sample tenants (runs once; `make up` and `make dev` do it too)
	@go build -o .dev/bin/hayat ./cmd/hayat
	@$(LOCAL) ./.dev/bin/hayat migrate
	@$(LOCAL) ./.dev/bin/hayat seed

db: docker      ## Postgres in Docker on 127.0.0.1:55432, and a master key kept in .dev/
	@mkdir -p .dev
	@[ -s .dev/master.key ] || (umask 077 && openssl rand -base64 32 > .dev/master.key)
	@$(COMPOSE) up -d --wait postgres

docker:         ## start Docker Desktop if its daemon isn't answering, and wait for it
	@docker info >/dev/null 2>&1 || { \
	  [ "$$(uname)" = Darwin ] && [ -d /Applications/Docker.app ] || { echo "Docker isn't running. Start it and retry."; exit 1; }; \
	  echo "Starting Docker Desktop…"; open -a Docker; \
	  for i in $$(seq 90); do docker info >/dev/null 2>&1 && exit 0; sleep 2; done; \
	  echo "Docker didn't come up within 3 minutes."; exit 1; }

db-down:        ## stop the local Postgres (its data stays in the compose volume)
	@$(COMPOSE) stop postgres

db-reset: docker ## wipe the local Postgres, so the next `make up` migrates and seeds from scratch
	@$(COMPOSE) rm -sfv postgres
	@docker volume rm -f compose_postgres >/dev/null

console:        ## the console at http://localhost:3000 (run `make dev` first)
	cd console && pnpm dev

test:           ## Go tests (embedded Postgres) + console typecheck
	go vet ./... && go test ./...
	cd console && pnpm typecheck

generate:       ## regenerate Go and TypeScript from proto/
	PATH="$$(go env GOPATH)/bin:$$PATH" buf lint && PATH="$$(go env GOPATH)/bin:$$PATH" buf generate

build:
	go build -o hayat ./cmd/hayat

images:
	docker build -t ghcr.io/aidapedia/hayat:dev .
	docker build -t ghcr.io/aidapedia/hayat-console:dev console
