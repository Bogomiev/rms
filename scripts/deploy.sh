#!/usr/bin/env bash
# Деплой RMS: обновление из git, пересборка образа, перезапуск контейнера.
# Используется как вручную (make deploy), так и из задания cron (make autodeploy).

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

FORCE=0
[ "${1:-}" = "--force" ] && FORCE=1

LOCK_FILE="$ROOT_DIR/.deploy.lock"
exec 200>"$LOCK_FILE"
if ! flock -n 200; then
    warn "Деплой уже выполняется другим процессом, выхожу."
    exit 0
fi

log_line() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" | tee -a "$DEPLOY_LOG"; }

if [ ! -f "$CONFIG_FILE" ]; then
    err "config/local.yaml не найден. Сначала выполните: make install"
    exit 1
fi

banner "RMS · Деплой"

step "Обновление из git"

BRANCH="$(git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")"
UPDATED=0

if [ -n "$BRANCH" ] && git -C "$ROOT_DIR" remote get-url origin >/dev/null 2>&1; then
    if git -C "$ROOT_DIR" fetch --quiet origin "$BRANCH"; then
        LOCAL="$(git -C "$ROOT_DIR" rev-parse HEAD)"
        REMOTE="$(git -C "$ROOT_DIR" rev-parse "origin/$BRANCH" 2>/dev/null || echo "$LOCAL")"
        if [ "$LOCAL" != "$REMOTE" ]; then
            log_line "Найдены изменения в origin/$BRANCH, выполняю git pull --ff-only"
            git -C "$ROOT_DIR" pull --ff-only origin "$BRANCH"
            UPDATED=1
        else
            log_line "Изменений в git нет."
        fi
    else
        warn "git fetch не удался (нет сети?), пропускаю обновление."
    fi
else
    warn "origin не настроен, пропускаю обновление из git."
fi

if [ "$UPDATED" -eq 0 ] && [ "$FORCE" -eq 0 ]; then
    ok "Обновлений нет, пересборка не требуется (используйте --force для принудительного деплоя)."
    exit 0
fi

step "Пересборка и перезапуск контейнера RMS"
ensure_shared_network
compose build rms
compose up -d rms

if wait_healthy rms-app 60; then
    ok "Деплой завершён, контейнер RMS отвечает на запросы."
    log_line "Деплой успешно завершён (commit: $(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "?"))."
else
    err "Контейнер RMS не прошёл проверку здоровья после деплоя."
    log_line "ОШИБКА: деплой завершён, но healthcheck не пройден."
    exit 1
fi
