#!/usr/bin/env bash
# Просмотр и переключение ветки деплоя (master/dev).
# Автодеплой (scripts/deploy.sh, запускаемый из cron) сам определяет текущую
# git-ветку через rev-parse --abbrev-ref HEAD, поэтому "ветка деплоя" — это
# просто та ветка, на которую переключён локальный checkout на сервере.
# Использование: scripts/branch.sh status | scripts/branch.sh set <branch>

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

ALLOWED_BRANCHES=(master dev)
CRON_MARKER="# rms-autodeploy"

repo_url() {
    git -C "$ROOT_DIR" remote get-url origin 2>/dev/null || echo "https://github.com/Bogomiev/rms"
}

current_branch() {
    git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "неизвестно"
}

autodeploy_status() {
    if command_exists crontab && crontab -l 2>/dev/null | grep -qF "$CRON_MARKER"; then
        echo "включён"
    else
        echo "выключен"
    fi
}

print_status() {
    local branch url ad_status
    branch="$(current_branch)"
    url="$(repo_url)"
    ad_status="$(autodeploy_status)"

    banner "RMS · Ветка деплоя"
    msg "  Репозиторий: ${C_BOLD}${url}${C_RESET}"
    msg "  Текущая ветка деплоя: ${C_BOLD}${branch}${C_RESET}  (cron автодеплоя следит именно за ней)"
    msg "  Автодеплой (cron): ${C_BOLD}${ad_status}${C_RESET}"
    echo

    if ! git -C "$ROOT_DIR" remote get-url origin >/dev/null 2>&1; then
        warn "origin не настроен, сравнение с GitHub недоступно."
        return
    fi

    step "Сравнение с GitHub"
    if ! git -C "$ROOT_DIR" fetch --quiet origin "$branch" 2>/dev/null; then
        warn "git fetch не удался (нет сети?), показываю последнее известное состояние."
    fi

    local remote_ref="origin/$branch"
    if ! git -C "$ROOT_DIR" rev-parse --verify "$remote_ref" >/dev/null 2>&1; then
        warn "Ветка '$branch' не найдена в $url."
    else
        local counts behind ahead
        counts="$(git -C "$ROOT_DIR" rev-list --left-right --count "${remote_ref}...HEAD")"
        read -r behind ahead <<<"$counts"
        if [ "$behind" = "0" ] && [ "$ahead" = "0" ]; then
            ok "Синхронизирована с $remote_ref."
        else
            [ "$behind" != "0" ] && warn "Отстаёт от $remote_ref на $behind коммит(ов) — подтянутся при следующем деплое."
            [ "$ahead" != "0" ] && warn "Локально есть $ahead коммит(ов), которых нет в $remote_ref."
        fi
    fi

    if [ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]; then
        warn "В рабочем каталоге есть незакоммиченные изменения."
    fi

    echo
    info "Переключить: make branch-master · make branch-dev"
}

set_branch() {
    local target="$1"
    local allowed=0 b
    for b in "${ALLOWED_BRANCHES[@]}"; do
        [ "$b" = "$target" ] && allowed=1
    done
    if [ "$allowed" -ne 1 ]; then
        err "Неизвестная ветка деплоя '$target'. Разрешены: ${ALLOWED_BRANCHES[*]}."
        exit 1
    fi

    banner "RMS · Переключение ветки деплоя на $target"

    if ! git -C "$ROOT_DIR" remote get-url origin >/dev/null 2>&1; then
        err "origin не настроен, переключение невозможно."
        exit 1
    fi

    if [ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]; then
        err "В рабочем каталоге есть незакоммиченные изменения. Закоммитьте их или сохраните через git stash перед переключением ветки."
        exit 1
    fi

    local current
    current="$(current_branch)"

    step "Получение веток из $(repo_url)"
    if ! git -C "$ROOT_DIR" fetch --quiet origin; then
        err "git fetch не удался (нет сети?)."
        exit 1
    fi

    if ! git -C "$ROOT_DIR" rev-parse --verify "origin/$target" >/dev/null 2>&1; then
        err "Ветка '$target' не найдена в $(repo_url). Убедитесь, что она создана и запушена на GitHub."
        exit 1
    fi

    if [ "$current" = "$target" ]; then
        ok "Уже на ветке '$target'."
    else
        step "Переключение на ветку '$target'"
        if git -C "$ROOT_DIR" show-ref --verify --quiet "refs/heads/$target"; then
            git -C "$ROOT_DIR" checkout "$target"
            git -C "$ROOT_DIR" merge --ff-only "origin/$target"
        else
            git -C "$ROOT_DIR" checkout -t "origin/$target"
        fi
        ok "Ветка деплоя переключена на '$target'."
    fi

    if command_exists crontab && crontab -l 2>/dev/null | grep -qF "$CRON_MARKER"; then
        info "Автодеплой уже настроен — cron будет обновлять и перезапускать именно ветку '$target' каждый час."
    else
        warn "Автодеплой не настроен. Выполните 'make autodeploy', чтобы cron следил за веткой '$target'."
    fi

    echo
    if confirm "Пересобрать и перезапустить контейнер RMS сейчас, чтобы применить ветку '$target' немедленно?"; then
        bash "$ROOT_DIR/scripts/deploy.sh" --force
    else
        info "Изменения применятся автодеплоем в течение часа, либо запустите 'make deploy' вручную."
    fi
}

ACTION="${1:-status}"
case "$ACTION" in
    status)
        print_status
        ;;
    set)
        TARGET="${2:-}"
        if [ -z "$TARGET" ]; then
            err "Использование: scripts/branch.sh set <branch>"
            exit 1
        fi
        set_branch "$TARGET"
        ;;
    *)
        err "Неизвестное действие '$ACTION'. Используйте: status | set <branch>"
        exit 1
        ;;
esac
