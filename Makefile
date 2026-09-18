SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.PHONY: help menu install deploy update start stop restart build logs status check autodeploy autodeploy-off down branch branch-master branch-dev rms-key

CYAN  := \033[36m
BOLD  := \033[1m
RESET := \033[0m

CURRENT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "неизвестно")
REPO_URL       := $(shell git remote get-url origin 2>/dev/null || echo "https://github.com/Bogomiev/rms")

help: ## Показать это меню команд
	@printf "$(CYAN)$(BOLD)\n"
	@printf "╔══════════════════════════════════════════════════════╗\n"
	@printf "║                RMS · Управление проектом              ║\n"
	@printf "╚══════════════════════════════════════════════════════╝\n"
	@printf "$(RESET)\n"
	@printf "  Репозиторий: $(REPO_URL)\n"
	@printf "  Текущая ветка деплоя: $(BOLD)$(CURRENT_BRANCH)$(RESET)  (cron автодеплоя следит именно за ней)\n\n"
	@awk 'BEGIN {FS = ":.*?## "} \
		/^##@/ { printf "$(BOLD)%s$(RESET)\n", substr($$0, 5); next } \
		/^[a-zA-Z0-9_-]+:.*?## / { printf "  $(CYAN)%-18s$(RESET) %s\n", $$1, $$2 }' \
		$(MAKEFILE_LIST)
	@printf "\nПодсказка: $(BOLD)make menu$(RESET) — выбрать команду из интерактивного меню.\n\n"

menu: ## Интерактивное меню (кнопки установки/деплоя/автодеплоя)
	@printf "$(CYAN)$(BOLD)\n  RMS · Меню\n$(RESET)\n"
	@printf "  Текущая ветка деплоя: $(BOLD)$(CURRENT_BRANCH)$(RESET)\n\n"
	@printf "  1) Установить\n"
	@printf "  2) Деплой (обновить и перезапустить)\n"
	@printf "  3) Настроить автодеплой (cron, интервал спросит)\n"
	@printf "  4) Отключить автодеплой\n"
	@printf "  5) Статус контейнеров\n"
	@printf "  6) Логи\n"
	@printf "  7) Ветка деплоя: статус\n"
	@printf "  8) Переключить ветку деплоя на master\n"
	@printf "  9) Переключить ветку деплоя на dev\n"
	@printf "  0) Выход\n\n"
	@read -r -p "Выберите пункт: " choice; \
	case "$$choice" in \
		1) $(MAKE) install ;; \
		2) $(MAKE) deploy ;; \
		3) $(MAKE) autodeploy ;; \
		4) $(MAKE) autodeploy-off ;; \
		5) $(MAKE) status ;; \
		6) $(MAKE) logs ;; \
		7) $(MAKE) branch ;; \
		8) $(MAKE) branch-master ;; \
		9) $(MAKE) branch-dev ;; \
		*) echo "Выход." ;; \
	esac

##@ Установка и запуск

install: ## Установить: проверить Docker, обновиться из git, настроить БД и запустить
	@bash scripts/install.sh

start: ## Запустить контейнеры (без пересборки)
	@bash -c 'source scripts/lib.sh; ensure_shared_network; compose up -d'

stop: ## Остановить контейнеры (без удаления)
	@bash -c 'source scripts/lib.sh; compose stop'

restart: stop start ## Перезапустить контейнеры

down: ## Остановить и удалить контейнеры (данные БД в volume сохранятся)
	@bash -c 'source scripts/lib.sh; compose down'

build: ## Пересобрать образ RMS без деплоя из git
	@bash -c 'source scripts/lib.sh; compose build rms'

##@ Обновление

deploy: ## Деплой: git pull, пересборка образа RMS и перезапуск контейнера
	@bash scripts/deploy.sh

update: deploy ## Синоним для deploy

##@ 🌿 Ветка деплоя (dev / master)

branch: ## Показать текущую ветку деплоя и её статус относительно GitHub
	@bash scripts/branch.sh status

branch-master: ## Переключить деплой на ветку master (прод) — cron автодеплоя пойдёт за ней
	@bash scripts/branch.sh set master

branch-dev: ## Переключить деплой на ветку dev — cron автодеплоя пойдёт за ней
	@bash scripts/branch.sh set dev

##@ Автодеплой

autodeploy: ## Настроить автодеплой из git через cron: спросить интервал (в минутах) проверки GitHub
	@bash scripts/autodeploy.sh on

autodeploy-off: ## Отключить автодеплой из cron
	@bash scripts/autodeploy.sh off

##@ Безопасность

rms-key: ## Настроить интеграцию с клиентом: RMS_SIGNING_KEY и RMS_APP_ORIGINS (config/local.yaml и .env)
	@bash scripts/rms-key.sh

##@ Диагностика

status: ## Проверка контейнеров и статуса healthcheck
	@bash scripts/check.sh

check: status ## Синоним для status

logs: ## Смотреть логи контейнеров (Ctrl+C для выхода)
	@bash -c 'source scripts/lib.sh; compose logs -f --tail=200'
