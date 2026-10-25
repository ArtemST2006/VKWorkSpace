GO ?= go
GOLANGCI_LINT ?= golangci-lint
ARGS ?=
BINARY ?= bin/ghreport

-include .env
export

.DEFAULT_GOAL := help
.PHONY: help install build run format lint test vet verify docker-build

help: ## Показать команды
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-15s %s\n", $$1, $$2}'

install: ## Скачать зависимости модуля
	$(GO) mod download

build: ## Собрать bin/ghreport
	@mkdir -p $(dir $(BINARY))
	$(GO) build -trimpath -o $(BINARY) ./cmd/ghreport

run: ## Запустить службу с аргументами ARGS
	$(GO) run ./cmd/ghreport $(ARGS)

format: ## Применить gofumpt и goimports как в AvitoLab
	$(GOLANGCI_LINT) fmt

lint: ## Проверить форматирование и код линтерами
	$(GO) mod tidy -diff
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) fmt --diff
	$(GOLANGCI_LINT) run

test: ## Тесты с race detector и покрытием
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html

vet: ## Проверка go vet
	$(GO) vet ./...

verify: ## Обязательные проверки ТЗ
	$(GO) test ./...
	$(GO) vet ./...
	@test -z "$$($(GO)fmt -l $$(find . -name '*.go' -type f))" || { echo 'Go files need formatting'; exit 1; }

docker-build: ## Собрать контейнер по ТЗ
	docker build -t ghreport .
