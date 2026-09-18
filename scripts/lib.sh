#!/usr/bin/env bash
# Общие функции для скриптов деплоя RMS.
# Подключается через: source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

set -euo pipefail

# --- Цвета -------------------------------------------------------------
if [ -t 1 ]; then
    C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'
    C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'
    C_BLUE=$'\033[34m'; C_CYAN=$'\033[36m'
else
    C_RESET=""; C_BOLD=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_CYAN=""
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/.env"
CONFIG_FILE="$ROOT_DIR/config/local.yaml"
EXAMPLE_CONFIG_FILE="$ROOT_DIR/config/example.yaml"
DEPLOY_LOG="$ROOT_DIR/deploy.log"

msg()      { printf '%s\n' "$*"; }
info()     { printf '%s[·]%s %s\n' "$C_BLUE" "$C_RESET" "$*"; }
ok()       { printf '%s[✔]%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn()     { printf '%s[!]%s %s\n' "$C_YELLOW" "$C_RESET" "$*"; }
err()      { printf '%s[✘]%s %s\n' "$C_RED" "$C_RESET" "$*" >&2; }
step()     { printf '\n%s%s%s\n' "$C_CYAN$C_BOLD" "▶ $*" "$C_RESET"; }

banner() {
    local title="$1"
    local width=54
    printf '\n%s' "$C_CYAN$C_BOLD"
    printf '╔'; printf '═%.0s' $(seq 1 "$width"); printf '╗\n'
    printf '║ %-*s ║\n' $((width - 2)) "$title"
    printf '╚'; printf '═%.0s' $(seq 1 "$width"); printf '╝'
    printf '%s\n' "$C_RESET"
}

confirm() {
    # confirm "Вопрос?" [y|n]  -- по умолчанию "y"
    local question="$1" default="${2:-y}" reply prompt
    if [ "$default" = "y" ]; then prompt="[Да/нет]"; else prompt="[да/Нет]"; fi
    read -r -p "$(printf '%s%s%s %s: ' "$C_YELLOW" "$question" "$C_RESET" "$prompt")" reply || true
    reply="${reply:-$default}"
    case "$reply" in
        [yYдД]*|"1") return 0 ;;
        *) return 1 ;;
    esac
}

ask() {
    # ask "Вопрос" "значение_по_умолчанию" -> печатает результат в stdout
    local question="$1" default="$2" reply
    read -r -p "$(printf '%s%s%s [%s]: ' "$C_YELLOW" "$question" "$C_RESET" "$default")" reply || true
    printf '%s' "${reply:-$default}"
}

command_exists() { command -v "$1" >/dev/null 2>&1; }

# --- сеть, общая с ecom_orders ------------------------------------------
# Оба проекта — отдельные docker-compose проекты со своими bridge-сетями,
# поэтому сервисы не видят друг друга по умолчанию. rms-ecom-shared —
# внешняя сеть-мост между ними (только сервисы rms/app, БД сюда не входит).
SHARED_NETWORK="rms-ecom-shared"

ensure_shared_network() {
    docker network inspect "$SHARED_NETWORK" >/dev/null 2>&1 || {
        info "Создаю сеть $SHARED_NETWORK (для связи с ecom_orders)..."
        docker network create "$SHARED_NETWORK" >/dev/null
    }
}

# --- docker compose (v2 плагин или отдельный docker-compose) -----------
compose() {
    if docker compose version >/dev/null 2>&1; then
        docker compose --env-file "$ENV_FILE" "$@"
    elif command_exists docker-compose; then
        docker-compose --env-file "$ENV_FILE" "$@"
    else
        err "docker compose не найден"
        exit 1
    fi
}

# --- .env ----------------------------------------------------------------
env_get() {
    # env_get KEY [default]
    local key="$1" default="${2:-}"
    if [ -f "$ENV_FILE" ]; then
        local line
        line="$(grep -E "^${key}=" "$ENV_FILE" 2>/dev/null | tail -n1 || true)"
        if [ -n "$line" ]; then
            printf '%s' "${line#*=}"
            return 0
        fi
    fi
    printf '%s' "$default"
}

env_set() {
    # env_set KEY VALUE — добавляет либо заменяет значение в .env
    local key="$1" value="$2" tmp
    touch "$ENV_FILE"
    tmp="$(mktemp)"
    local found=0
    while IFS= read -r line || [ -n "$line" ]; do
        if [[ "$line" == "${key}="* ]]; then
            printf '%s=%s\n' "$key" "$value" >> "$tmp"
            found=1
        else
            printf '%s\n' "$line" >> "$tmp"
        fi
    done < "$ENV_FILE"
    if [ "$found" -eq 0 ]; then
        printf '%s=%s\n' "$key" "$value" >> "$tmp"
    fi
    mv "$tmp" "$ENV_FILE"
    chmod 600 "$ENV_FILE"
}

# --- Правка config/local.yaml без regex-экранирования секретов ----------
# Значение подставляется через printf %s — без интерпретации спецсимволов.
yaml_set_nested() {
    # yaml_set_nested секция ключ значение
    local section="$1" key="$2" value="$3" file="$CONFIG_FILE" tmp cur_section=""
    tmp="$(mktemp)"
    while IFS= read -r line || [ -n "$line" ]; do
        if [[ "$line" =~ ^[A-Za-z_] ]]; then
            cur_section="${line%%:*}"
        fi
        if [ "$cur_section" = "$section" ] && [[ "$line" =~ ^\ \ ${key}:(\ |$) ]]; then
            printf '  %s: %s\n' "$key" "$value" >> "$tmp"
        else
            printf '%s\n' "$line" >> "$tmp"
        fi
    done < "$file"
    mv "$tmp" "$file"
}

yaml_set_root() {
    # yaml_set_root ключ значение_в_кавычках_если_нужно
    # Если ключа нет в файле — строка дописывается в конец.
    local key="$1" value="$2" file="$CONFIG_FILE" tmp found=0
    tmp="$(mktemp)"
    while IFS= read -r line || [ -n "$line" ]; do
        if [[ "$line" =~ ^${key}: ]]; then
            printf '%s: %s\n' "$key" "$value" >> "$tmp"
            found=1
        else
            printf '%s\n' "$line" >> "$tmp"
        fi
    done < "$file"
    if [ "$found" -eq 0 ]; then
        printf '%s: %s\n' "$key" "$value" >> "$tmp"
    fi
    mv "$tmp" "$file"
}

yaml_dquote() {
    # Экранирует \ и " и оборачивает значение в двойные кавычки для YAML.
    local s="$1"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    printf '"%s"' "$s"
}

byte_length() {
    printf '%s' "$1" | wc -c
}

random_hex() {
    local bytes="${1:-16}"
    if command_exists openssl; then
        openssl rand -hex "$bytes"
    else
        head -c "$bytes" /dev/urandom | od -An -tx1 | tr -d ' \n'
    fi
}

wait_healthy() {
    # wait_healthy имя_контейнера таймаут_в_секундах
    local name="$1" timeout="${2:-60}" waited=0 status
    while [ "$waited" -lt "$timeout" ]; do
        status="$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo "нет")"
        if [ "$status" = "healthy" ]; then
            return 0
        fi
        sleep 2
        waited=$((waited + 2))
    done
    return 1
}
