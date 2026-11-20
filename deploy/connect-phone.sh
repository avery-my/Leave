#!/usr/bin/env bash
set -eu

fail() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

adb=${ADB_PATH:-adb}
command -v "$adb" >/dev/null || fail 'ADB was not found. Install Android Platform Tools or set ADB_PATH.'

device_list=$("$adb" devices) || fail 'Could not list ADB devices.'
authorized=$(printf '%s\n' "$device_list" | awk '$2 == "device" { print $1 }')
serial=${DEVICE_SERIAL:-}

if [[ -z "$serial" ]]; then
    if [[ -z "$authorized" || "$authorized" == *$'\n'* ]]; then
        printf '%s\n' "$device_list" >&2
        fail 'Connect and authorize one device, or pass DEVICE_SERIAL.'
    fi
    serial=$authorized
elif ! printf '%s\n' "$authorized" | grep -Fxq -- "$serial"; then
    fail "Device $serial is not connected and authorized."
fi

mapping_target() {
    awk -v endpoint="$1" 'NF >= 2 && $(NF-1) == endpoint { print $NF }'
}

forward_port() {
    local port=$1 endpoint="tcp:$1" mappings target
    if ! mappings=$("$adb" -s "$serial" reverse --list); then
        printf 'Could not read existing mappings.\n' >&2
        return 1
    fi
    target=$(printf '%s\n' "$mappings" | mapping_target "$endpoint")
    if [[ -n "$target" && "$target" != "$endpoint" ]]; then
        printf '%s already maps to %s.\n' "$endpoint" "$target" >&2
        return 1
    fi
    if ! nc -z -w 2 127.0.0.1 "$port"; then
        printf 'No local service reachable on port %s.\n' "$port" >&2
        return 1
    fi
    # --no-rebind also protects a mapping created after the listing above.
    if [[ -z "$target" ]] && ! "$adb" -s "$serial" reverse --no-rebind "$endpoint" "$endpoint"; then
        printf 'Could not forward %s.\n' "$endpoint" >&2
        return 1
    fi
    if ! mappings=$("$adb" -s "$serial" reverse --list); then
        printf 'Could not verify port mappings.\n' >&2
        return 1
    fi
    if [[ $(printf '%s\n' "$mappings" | mapping_target "$endpoint") != "$endpoint" ]]; then
        printf 'Expected mapping for %s is missing.\n' "$endpoint" >&2
        return 1
    fi
    printf 'USB forwarding ready for %s on port %s.\n' "$serial" "$port"
}

forward_port 8080 || exit 1
if ! forward_port 8081; then
    printf 'Skipping optional avatar port 8081.\n' >&2
fi
