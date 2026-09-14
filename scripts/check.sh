#!/usr/bin/env bash
# Проверка состояния контейнеров RMS.

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

banner "RMS · Проверка контейнеров"

if [ ! -f "$ENV_FILE" ]; then
    warn "Файл .env не найден. Похоже, установка ещё не выполнялась (make install)."
fi

PROFILE_ARGS=()
if [ "$(env_get COMPOSE_PROFILES "")" = "local-db" ]; then
    PROFILE_ARGS=(--profile local-db)
fi

compose "${PROFILE_ARGS[@]}" ps

for name in rms-app rms-postgres; do
    if docker inspect "$name" >/dev/null 2>&1; then
        status="$(docker inspect -f '{{.State.Status}}' "$name")"
        health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}нет healthcheck{{end}}' "$name")"
        if [ "$status" = "running" ] && { [ "$health" = "healthy" ] || [ "$health" = "нет healthcheck" ]; }; then
            ok "$name: статус=$status здоровье=$health"
        else
            warn "$name: статус=$status здоровье=$health"
        fi
    else
        info "$name: контейнер не найден (не запущен)"
    fi
done

APP_PORT="$(env_get APP_PORT 8082)"
step "Проверка HTTP-ответа RMS (порт $APP_PORT)"
if curl -s -o /dev/null -w '%{http_code}' "http://localhost:${APP_PORT}/usertokenvalid?token=healthcheck" | grep -qE '^[0-9]+$'; then
    ok "RMS отвечает на http://localhost:${APP_PORT}"
else
    warn "RMS не отвечает на http://localhost:${APP_PORT}"
fi
