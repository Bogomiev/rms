#!/usr/bin/env bash
# Настройка интеграции с ecom_orders (или другим клиентом RMS): RMS_SIGNING_KEY
# и разрешённые origin (RMS_APP_ORIGINS) — те же две настройки, что клиент
# просит на своей стороне командой `make rms`. Сохраняет обе в config/local.yaml
# и .env, т.к. docker-compose.yml требует переменные окружения для контейнера.

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

banner "RMS · Интеграция (RMS_SIGNING_KEY, RMS_APP_ORIGINS)"

if [ ! -f "$CONFIG_FILE" ]; then
    cp "$EXAMPLE_CONFIG_FILE" "$CONFIG_FILE"
    chmod 600 "$CONFIG_FILE"
    ok "config/local.yaml создан из config/example.yaml."
fi

CUR_YAML="$(grep -E '^signing_key:[[:space:]]*' "$CONFIG_FILE" 2>/dev/null | sed -E 's/^signing_key:[[:space:]]*"?//; s/"?[[:space:]]*$//' || true)"
CUR_ENV="$(env_get RMS_SIGNING_KEY "")"
DEFAULT_KEY="${CUR_YAML:-${CUR_ENV:-$(random_hex 32)}}"

while true; do
    SIGNING_KEY="$(ask "RMS_SIGNING_KEY (мин. 32 байта)" "$DEFAULT_KEY")"
    LEN="$(byte_length "$SIGNING_KEY")"
    if [ "$LEN" -lt 32 ]; then
        warn "Ключ должен содержать минимум 32 байта (сейчас: $LEN байт). Попробуйте ещё раз."
        continue
    fi
    break
done

yaml_set_root signing_key "$(yaml_dquote "$SIGNING_KEY")"
ok "signing_key записан в config/local.yaml."

env_set RMS_SIGNING_KEY "$SIGNING_KEY"
ok "RMS_SIGNING_KEY сохранён в .env (используется docker-compose.yml для контейнера)."

# --- Разрешённые origin (RMS_APP_ORIGINS) ---------------------------------
# ВАЖНО: docker-compose.yml ВСЕГДА передаёт RMS_APP_ORIGINS в контейнер
# (с дефолтом, если её нет в .env), а конфиг (internal/config/config.go)
# отдаёт переменной окружения приоритет над app_origins из config/local.yaml.
# Поэтому редактировать app_origins прямо в yaml бесполезно — источник
# истины именно .env, и обновлять его нужно здесь.
step "Разрешённые origin (RMS_APP_ORIGINS)"
info "Это публичный адрес БРАУЗЕРА клиента (его APP_ORIGIN), а не адрес RMS."
info "Пример: https://shop.example.com — то же значение, что клиент указал у себя в APP_ORIGIN."
CUR_ORIGINS="$(env_get RMS_APP_ORIGINS "http://localhost:3000,http://localhost:8082")"
APP_ORIGINS="$(ask "Список origin через запятую" "$CUR_ORIGINS")"
env_set RMS_APP_ORIGINS "$APP_ORIGINS"
ok "RMS_APP_ORIGINS сохранён в .env."

warn "Если контейнер RMS уже запущен — перезапустите его: make restart"
