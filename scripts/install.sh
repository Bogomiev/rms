#!/usr/bin/env bash
# Установка RMS: проверка Docker, обновление из git, подготовка конфигурации,
# выбор PostgreSQL (свой контейнер / внешний сервер), сборка и запуск.

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

banner "RMS · Установка"

# 1. Docker и Docker Compose ------------------------------------------------
step "Проверка Docker"

if command_exists docker; then
    ok "Docker найден: $(docker --version)"
else
    warn "Docker не найден."
    if [ -f /etc/debian_version ] && confirm "Установить Docker через официальный скрипт get.docker.com?"; then
        curl -fsSL https://get.docker.com | sh
        if command_exists docker; then
            ok "Docker установлен."
            warn "Возможно потребуется перелогиниться, чтобы пользователь входил в группу docker без sudo."
        else
            err "Установка Docker не удалась. Установите вручную: https://docs.docker.com/engine/install/"
            exit 1
        fi
    else
        err "Docker обязателен для установки RMS."
        msg "Инструкция по установке: https://docs.docker.com/engine/install/"
        exit 1
    fi
fi

if docker compose version >/dev/null 2>&1; then
    ok "Docker Compose найден: $(docker compose version --short 2>/dev/null || echo v2)"
elif command_exists docker-compose; then
    ok "Docker Compose найден: $(docker-compose --version)"
else
    warn "Docker Compose не найден."
    if [ -f /etc/debian_version ] && command_exists apt-get && confirm "Установить пакет docker-compose-plugin через apt?"; then
        sudo apt-get update -y && sudo apt-get install -y docker-compose-plugin
    fi
    if ! docker compose version >/dev/null 2>&1 && ! command_exists docker-compose; then
        err "Docker Compose обязателен. Инструкция: https://docs.docker.com/compose/install/"
        exit 1
    fi
    ok "Docker Compose установлен."
fi

# 2. Обновление из git -------------------------------------------------------
step "Проверка обновлений из репозитория"

REPO_URL="https://github.com/Bogomiev/rms"

if [ -d "$ROOT_DIR/.git" ]; then
    if ! git -C "$ROOT_DIR" remote get-url origin >/dev/null 2>&1; then
        info "Удалённый репозиторий origin не настроен, добавляю $REPO_URL"
        git -C "$ROOT_DIR" remote add origin "$REPO_URL"
    fi
    BRANCH="$(git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD)"
    if git -C "$ROOT_DIR" fetch --quiet origin "$BRANCH" 2>/dev/null; then
        LOCAL="$(git -C "$ROOT_DIR" rev-parse HEAD)"
        REMOTE="$(git -C "$ROOT_DIR" rev-parse "origin/$BRANCH" 2>/dev/null || echo "$LOCAL")"
        if [ "$LOCAL" != "$REMOTE" ]; then
            warn "Доступны обновления в ветке $BRANCH."
            if confirm "Подтянуть последние изменения (git pull --ff-only)?"; then
                git -C "$ROOT_DIR" pull --ff-only origin "$BRANCH"
                ok "Репозиторий обновлён."
            fi
        else
            ok "Репозиторий уже актуален."
        fi
    else
        warn "Не удалось выполнить git fetch (нет сети или доступа к $REPO_URL). Продолжаю с текущей версией."
    fi
else
    warn "Каталог не является git-репозиторием, пропускаю проверку обновлений."
fi

# 3. Конфигурация -------------------------------------------------------------
step "Подготовка config/local.yaml"

if [ -f "$CONFIG_FILE" ]; then
    warn "Файл config/local.yaml уже существует."
    if confirm "Пересоздать его из config/example.yaml? (текущие настройки будут потеряны)" n; then
        cp "$EXAMPLE_CONFIG_FILE" "$CONFIG_FILE"
        ok "config/local.yaml пересоздан из шаблона."
    else
        ok "Использую существующий config/local.yaml."
    fi
else
    cp "$EXAMPLE_CONFIG_FILE" "$CONFIG_FILE"
    ok "config/local.yaml скопирован из config/example.yaml."
fi
chmod 600 "$CONFIG_FILE"

info "Переменная запуска (см. README.md): export CONFIG_PATH=./config/local.yaml"
info "В контейнере она уже прописана в образе (CONFIG_PATH=/app/config/local.yaml)."

# 4. PostgreSQL: свой контейнер или внешний сервер -----------------------------
step "Настройка PostgreSQL"

CUR_MODE="$(env_get COMPOSE_PROFILES "")"
DEFAULT_MODE="own"
[ "$CUR_MODE" != "local-db" ] && [ -n "$CUR_MODE" ] && DEFAULT_MODE="external"

msg "  ${C_BOLD}1${C_RESET}) Поднять собственный контейнер PostgreSQL (по умолчанию)"
msg "  ${C_BOLD}2${C_RESET}) Подключиться к уже существующему серверу PostgreSQL"
DB_CHOICE="$(ask "Выберите вариант (1/2)" "1")"

if [ "$DB_CHOICE" = "2" ]; then
    DB_MODE="external"
    env_set COMPOSE_PROFILES ""
    DB_HOST="$(ask "Адрес внешнего сервера PostgreSQL (host.docker.internal — если сервер на этой же машине)" "$(env_get DB_HOST "host.docker.internal")")"
    DB_PORT_APP="$(ask "Порт PostgreSQL" "$(env_get DB_PORT 5432)")"
    DB_NAME="$(ask "Имя базы данных" "$(env_get DB_NAME rms)")"
    DB_USER="$(ask "Пользователь БД" "$(env_get DB_USER rms)")"
    while true; do
        read -r -s -p "$(printf '%sПароль пользователя БД на внешнем сервере%s: ' "$C_YELLOW" "$C_RESET")" DB_PASSWORD; echo
        if [ -n "$DB_PASSWORD" ]; then
            break
        fi
        warn "У внешнего сервера PostgreSQL не может быть пустого пароля — введите пароль, который уже настроен на нём."
    done
    DB_SSLMODE="$(ask "sslmode (disable/require/verify-ca/verify-full)" "$(env_get DB_SSLMODE disable)")"

    env_set DB_HOST "$DB_HOST"
    env_set DB_PORT "$DB_PORT_APP"
    env_set DB_NAME "$DB_NAME"
    env_set DB_USER "$DB_USER"
    env_set DB_PASSWORD "$DB_PASSWORD"
    env_set DB_SSLMODE "$DB_SSLMODE"

    yaml_set_nested db host "$DB_HOST"
    yaml_set_nested db port "$DB_PORT_APP"
    yaml_set_nested db user "$DB_USER"
    yaml_set_nested db password "$DB_PASSWORD"
    yaml_set_nested db dbname "$DB_NAME"
    yaml_set_nested db sslmode "$DB_SSLMODE"
    ok "Будет использован внешний сервер PostgreSQL: $DB_HOST:$DB_PORT_APP"
else
    DB_MODE="own"
    env_set COMPOSE_PROFILES "local-db"
    DB_NAME="$(ask "Имя базы данных" "$(env_get DB_NAME rms)")"
    DB_USER="$(ask "Пользователь БД" "$(env_get DB_USER rms)")"
    EXISTING_PWD="$(env_get DB_PASSWORD "")"
    DB_PASSWORD="$(ask "Пароль БД (Enter — оставить текущий или сгенерировать новый)" "$EXISTING_PWD")"
    if [ -z "$DB_PASSWORD" ]; then
        DB_PASSWORD="$(random_hex 16)"
        DB_PASSWORD_SOURCE="сгенерирован автоматически"
    elif [ -n "$EXISTING_PWD" ] && [ "$DB_PASSWORD" = "$EXISTING_PWD" ]; then
        DB_PASSWORD_SOURCE="сохранён из предыдущей установки (.env)"
    else
        DB_PASSWORD_SOURCE="указан вами"
    fi
    DB_PORT_HOST="$(ask "Порт PostgreSQL на хостовой машине" "$(env_get DB_PORT 5432)")"

    env_set DB_NAME "$DB_NAME"
    env_set DB_USER "$DB_USER"
    env_set DB_PASSWORD "$DB_PASSWORD"
    env_set DB_PORT "$DB_PORT_HOST"

    # Внутри docker-сети контейнер rms обращается к postgres по имени сервиса.
    yaml_set_nested db host "postgres"
    yaml_set_nested db port "5432"
    yaml_set_nested db user "$DB_USER"
    yaml_set_nested db password "$DB_PASSWORD"
    yaml_set_nested db dbname "$DB_NAME"
    yaml_set_nested db sslmode "disable"
    ok "Будет поднят собственный контейнер PostgreSQL (порт на хосте: $DB_PORT_HOST)."

    banner "Данные для входа в PostgreSQL"
    msg "  Хост изнутри docker-сети: ${C_BOLD}postgres${C_RESET} (для контейнера rms)"
    msg "  Хост с этой машины:       ${C_BOLD}localhost:${DB_PORT_HOST}${C_RESET}"
    msg "  База данных:              ${C_BOLD}${DB_NAME}${C_RESET}"
    msg "  Пользователь:             ${C_BOLD}${DB_USER}${C_RESET}"
    msg "  Пароль:                   ${C_BOLD}${DB_PASSWORD}${C_RESET} (${DB_PASSWORD_SOURCE})"
    warn "Этот пароль передаётся контейнеру PostgreSQL при первом запуске (POSTGRES_PASSWORD) и записан в config/local.yaml и .env."
fi

# 5. Первый администратор ------------------------------------------------------
step "Первый администратор (first_admin_pwd)"
info "Пароль нужен только для пустой БД: сервер создаст пользователя admin с этим паролем."
info "Пустой пароль не допускается — сервер не запустится (проверено: 8..72 байта)."

FIRST_ADMIN_PWD=""
while true; do
    INPUT="$(ask "Пароль первого администратора, 8-72 байт (Enter — сгенерировать случайный)" "")"
    if [ -z "$INPUT" ]; then
        FIRST_ADMIN_PWD="$(random_hex 12)"
        ok "Сгенерирован случайный пароль администратора."
        break
    fi
    LEN="$(byte_length "$INPUT")"
    if [ "$LEN" -lt 8 ] || [ "$LEN" -gt 72 ]; then
        warn "Пароль должен содержать от 8 до 72 байт (сейчас: $LEN байт). Попробуйте ещё раз."
        continue
    fi
    FIRST_ADMIN_PWD="$INPUT"
    break
done

yaml_set_root first_admin_pwd "$(yaml_dquote "$FIRST_ADMIN_PWD")"

banner "Данные для входа администратора"
msg "  Логин (user_token): ${C_BOLD}admin${C_RESET}"
msg "  Пароль:             ${C_BOLD}${FIRST_ADMIN_PWD}${C_RESET}"
warn "Запишите пароль сейчас — повторно он нигде не выводится и не сохраняется отдельно."
info "Пароль применится, только если таблица пользователей пуста (первый запуск на новой БД)."

# 6. Ключ подписи JWT -----------------------------------------------------------
step "Ключ подписи JWT (RMS_SIGNING_KEY)"
SIGNING_KEY="$(env_get RMS_SIGNING_KEY "")"
if [ -z "$SIGNING_KEY" ]; then
    SIGNING_KEY="$(random_hex 32)"
    env_set RMS_SIGNING_KEY "$SIGNING_KEY"
    ok "Сгенерирован новый RMS_SIGNING_KEY и сохранён в .env."
else
    ok "Использую существующий RMS_SIGNING_KEY из .env (сессии пользователей сохранятся)."
fi

# 7. Разрешённые origin (app_origins) --------------------------------------------
step "Разрешённые origin приложения (app_origins)"
CUR_ORIGINS="$(env_get RMS_APP_ORIGINS "http://localhost:3000,http://localhost:8082")"
APP_ORIGINS="$(ask "Список origin через запятую" "$CUR_ORIGINS")"
env_set RMS_APP_ORIGINS "$APP_ORIGINS"
ok "app_origins будет применён через переменную окружения контейнера RMS_APP_ORIGINS."

APP_PORT="$(ask "Порт RMS на хостовой машине" "$(env_get APP_PORT 8082)")"
env_set APP_PORT "$APP_PORT"

# 8. Сборка и запуск ------------------------------------------------------------
step "Сборка и запуск контейнеров"

if [ "$DB_MODE" = "own" ]; then
    info "Запускаю контейнер PostgreSQL..."
    compose --profile local-db up -d postgres
    if wait_healthy rms-postgres 60; then
        ok "PostgreSQL готов."
    else
        err "PostgreSQL не перешёл в состояние healthy за отведённое время."
        exit 1
    fi
fi

info "Собираю и запускаю образ RMS..."
compose up -d --build rms

if wait_healthy rms-app 60; then
    ok "Контейнер RMS отвечает на запросы."
else
    warn "Контейнер RMS запущен, но проверка здоровья пока не прошла. Смотрите: make logs"
fi

banner "Установка завершена"
ok "RMS доступен на http://localhost:${APP_PORT}"
info "Полезные команды: make status · make logs · make deploy · make autodeploy"
