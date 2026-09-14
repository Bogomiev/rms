SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.PHONY: help menu install deploy update start stop restart build logs status check autodeploy autodeploy-off down

CYAN  := \033[36m
BOLD  := \033[1m
RESET := \033[0m

help: ## Показать это меню команд
	@printf "$(CYAN)$(BOLD)\n"
	@printf "╔══════════════════════════════════════════════════════╗\n"
	@printf "║                RMS · Управление проектом              ║\n"
	@printf "╚══════════════════════════════════════════════════════╝\n"
	@printf "$(RESET)\n"
	@awk 'BEGIN {FS = ":.*?## "} \
		/^##@/ { printf "$(BOLD)%s$(RESET)\n", substr($$0, 5); next } \
		/^[a-zA-Z0-9_-]+:.*?## / { printf "  $(CYAN)%-18s$(RESET) %s\n", $$1, $$2 }' \
		$(MAKEFILE_LIST)
	@printf "\nПодсказка: $(BOLD)make menu$(RESET) — выбрать команду из интерактивного меню.\n\n"

menu: ## Интерактивное меню (кнопки установки/деплоя/автодеплоя)
	@printf "$(CYAN)$(BOLD)\n  RMS · Меню\n$(RESET)\n"
	@printf "  1) Установить\n"
	@printf "  2) Деплой (обновить и перезапустить)\n"
	@printf "  3) Настроить автодеплой (cron, раз в час)\n"
	@printf "  4) Отключить автодеплой\n"
	@printf "  5) Статус контейнеров\n"
	@printf "  6) Логи\n"
	@printf "  0) Выход\n\n"
	@read -r -p "Выберите пункт: " choice; \
	case "$$choice" in \
		1) $(MAKE) install ;; \
		2) $(MAKE) deploy ;; \
		3) $(MAKE) autodeploy ;; \
		4) $(MAKE) autodeploy-off ;; \
		5) $(MAKE) status ;; \
		6) $(MAKE) logs ;; \
		*) echo "Выход." ;; \
	esac

##@ Установка и запуск

install: ## Установить: проверить Docker, обновиться из git, настроить БД и запустить
	@bash scripts/install.sh

start: ## Запустить контейнеры (без пересборки)
	@bash -c 'source scripts/lib.sh; compose up -d'

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

##@ Автодеплой

autodeploy: ## Настроить автодеплой из git через cron (раз в час)
	@bash scripts/autodeploy.sh on

autodeploy-off: ## Отключить автодеплой из cron
	@bash scripts/autodeploy.sh off

##@ Диагностика

status: ## Проверка контейнеров и статуса healthcheck
	@bash scripts/check.sh

check: status ## Синоним для status

logs: ## Смотреть логи контейнеров (Ctrl+C для выхода)
	@bash -c 'source scripts/lib.sh; compose logs -f --tail=200'
