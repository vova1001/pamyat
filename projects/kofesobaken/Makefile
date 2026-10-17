# КофеСобакен — команды разработки. Всё запускается в Docker.
SHELL := /bin/sh
COMPOSE := docker compose

.PHONY: env dev up down logs test e2e lint gen reset

## .env из .env.example со случайными паролями (если .env ещё нет)
env:
	@if [ ! -f .env ]; then \
	  cp .env.example .env; \
	  sed -i "s/^ADMIN_PASSWORD=.*/ADMIN_PASSWORD=$$(head -c 12 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 14)/" .env; \
	  sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 20)/" .env; \
	  sed -i "s/^PAYMENT_WEBHOOK_SECRET=.*/PAYMENT_WEBHOOK_SECRET=$$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')/" .env; \
	  echo ".env создан — пароль кабинета в ADMIN_PASSWORD"; \
	else echo ".env уже есть"; fi

## поднять всё (сайт http://localhost:8000, кабинет /admin)
dev: env
	$(COMPOSE) up --build -d
	@echo "Сайт: http://localhost:$${WEB_PORT:-8000}   Кабинет: http://localhost:$${WEB_PORT:-8000}/admin"

up: dev

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f backend

## тесты бэка (модульные и интеграционные на PostgreSQL) — в Docker
test:
	$(COMPOSE) --profile test run --rm --build backend-test; s=$$?; $(COMPOSE) --profile test rm -sf db-test >/dev/null 2>&1; exit $$s

## e2e главных сценариев (Playwright) против поднятого стека; нужен make dev
e2e:
	@set -a; . ./.env; set +a; cd frontend && npx playwright test

## проверки кода
lint:
	cd backend && gofmt -l . | (! grep .) && go vet ./...
	cd frontend && npx astro check

## типы фронта и код store из контрактов
gen:
	cd backend && sqlc generate
	cd frontend && npm run gen:api

## стереть данные (база и фото) — необратимо
reset:
	$(COMPOSE) down -v
