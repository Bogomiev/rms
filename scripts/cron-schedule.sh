#!/usr/bin/env bash
# Печатает поля "минута час" crontab-строки для интервала автодеплоя,
# заданного в минутах. Используется из scripts/autodeploy.sh (`make autodeploy`).
#
# < 60          -> "*/N *"      (каждые N минут)
# кратно 60     -> "0 */H"      (каждые H часов; H=1 -> "0 *")
# иначе         -> ошибка (в стандартном cron не выразить одной строкой)

set -euo pipefail

MIN="${1:?Usage: cron-schedule.sh <minutes>}"

if ! [[ "$MIN" =~ ^[0-9]+$ ]] || [ "$MIN" -le 0 ]; then
    echo "интервал должен быть целым числом минут больше 0 (получено: $MIN)" >&2
    exit 1
fi

if [ "$MIN" -lt 60 ]; then
    echo "*/$MIN *"
elif [ $(( MIN % 60 )) -eq 0 ]; then
    HOURS=$(( MIN / 60 ))
    if [ "$HOURS" -eq 1 ]; then
        echo "0 *"
    else
        echo "0 */$HOURS"
    fi
else
    echo "интервал $MIN мин не выразить одной cron-строкой — введите значение меньше 60 либо кратное 60 (120, 180, ...)" >&2
    exit 1
fi
