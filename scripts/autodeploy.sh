#!/usr/bin/env bash
# Настройка автодеплоя через cron: обновление из git и перезапуск с заданным
# пользователем интервалом. Использование: scripts/autodeploy.sh on|off
# Интервал (в минутах) хранится в .env (AUTODEPLOY_INTERVAL_MIN), чтобы при
# повторном запуске 'make autodeploy' можно было просто подтвердить его Enter'ом.

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

ACTION="${1:-on}"
MARKER="# rms-autodeploy"
CRON_SCHEDULE_SCRIPT="$(dirname "${BASH_SOURCE[0]}")/cron-schedule.sh"
AUTODEPLOY_INTERVAL_DEFAULT=60

remove_existing() {
    crontab -l 2>/dev/null | grep -vF "$MARKER" || true
}

if [ "$ACTION" = "off" ]; then
    banner "RMS · Отключение автодеплоя"
    if ! command_exists crontab; then
        warn "crontab не найден, отключать нечего."
        exit 0
    fi
    NEW_CRON="$(remove_existing)"
    printf '%s\n' "$NEW_CRON" | crontab -
    ok "Задание автодеплоя удалено из crontab."
    exit 0
fi

banner "RMS · Настройка автодеплоя"

step "Проверка cron"
if command_exists crontab; then
    ok "crontab найден."
elif [ -f /etc/debian_version ] && command_exists apt-get; then
    warn "crontab не найден."
    if confirm "Установить пакет cron через apt?"; then
        sudo apt-get update -y && sudo apt-get install -y cron
        sudo systemctl enable --now cron 2>/dev/null || sudo service cron start 2>/dev/null || true
    fi
    if ! command_exists crontab; then
        err "Не удалось установить cron. Настройте автодеплой вручную."
        exit 1
    fi
    ok "cron установлен."
else
    err "crontab не найден и автоматическая установка недоступна на этой системе."
    msg "Настройте планировщик вручную, чтобы раз в час выполнялся: $ROOT_DIR/scripts/deploy.sh"
    exit 1
fi

step "Интервал проверки GitHub"
CURRENT_MIN="$(env_get AUTODEPLOY_INTERVAL_MIN "$AUTODEPLOY_INTERVAL_DEFAULT")"
while true; do
    MIN="$(ask "Интервал проверки GitHub, в минутах (<60 — каждые N минут; кратно 60 — каждые N/60 часов)" "$CURRENT_MIN")"
    if SCHEDULE="$("$CRON_SCHEDULE_SCRIPT" "$MIN" 2>&1)"; then
        break
    fi
    warn "$SCHEDULE"
done
env_set AUTODEPLOY_INTERVAL_MIN "$MIN"

step "Установка задания в crontab (каждые $MIN мин)"
# $SCHEDULE — это "минута час" (2 поля); cron ждёт ровно 5 полей времени
# (минута час день-месяца месяц день-недели) перед командой — не забыть
# все 3 оставшихся "*", иначе crontab откажет с "bad day-of-week".
CRON_LINE="$SCHEDULE * * * cd $ROOT_DIR && $ROOT_DIR/scripts/deploy.sh >> $DEPLOY_LOG 2>&1 $MARKER — интервал: ${MIN} мин"
NEW_CRON="$(remove_existing)"
{
    [ -n "$NEW_CRON" ] && printf '%s\n' "$NEW_CRON"
    printf '%s\n' "$CRON_LINE"
} | crontab -

ok "Автодеплой настроен: обновление из git и перезапуск контейнера RMS каждые $MIN мин."
info "Лог автодеплоя: $DEPLOY_LOG"
info "Изменить интервал позже: make autodeploy (спросит заново, текущее значение — по умолчанию)"
info "Отключить: make autodeploy-off"
