#!/usr/bin/env bash
# Настройка автодеплоя через cron: обновление из git и перезапуск раз в час.
# Использование: scripts/autodeploy.sh on|off

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

ACTION="${1:-on}"
MARKER="# rms-autodeploy"
CRON_LINE="0 * * * * cd $ROOT_DIR && $ROOT_DIR/scripts/deploy.sh >> $DEPLOY_LOG 2>&1 $MARKER"

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

step "Установка задания в crontab (раз в час)"
NEW_CRON="$(remove_existing)"
{
    [ -n "$NEW_CRON" ] && printf '%s\n' "$NEW_CRON"
    printf '%s\n' "$CRON_LINE"
} | crontab -

ok "Автодеплой настроен: обновление из git и перезапуск контейнера RMS каждый час."
info "Лог автодеплоя: $DEPLOY_LOG"
info "Отключить: make autodeploy-off"
