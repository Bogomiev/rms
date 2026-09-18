#!/usr/bin/env bash
# Задать RMS_SIGNING_KEY: спросить значение у пользователя и сохранить
# его в config/local.yaml (signing_key), а также синхронизировать .env,
# так как docker-compose.yml требует переменную RMS_SIGNING_KEY.

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$ROOT_DIR"

banner "RMS · RMS_SIGNING_KEY"

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

warn "Если контейнер RMS уже запущен — перезапустите его: make restart"
