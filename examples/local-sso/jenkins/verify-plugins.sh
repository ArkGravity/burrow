#!/bin/sh
set -eu
plugins=/usr/share/jenkins/ref/plugins
lock=/usr/share/jenkins/ref/plugins.txt
expected=0
while IFS=: read -r plugin version; do
    [ -n "$plugin" ] || continue
    actual=$(unzip -p "$plugins/$plugin.jpi" META-INF/MANIFEST.MF | tr -d '\r' | sed -n 's/^Plugin-Version: //p')
    if [ "$actual" != "$version" ]; then
        echo "Plugin version differs from lock: $plugin" >&2
        exit 1
    fi
    expected=$((expected + 1))
done < "$lock"
actual_count=$(find "$plugins" -maxdepth 1 -name '*.jpi' | wc -l | tr -d ' ')
[ "$actual_count" -eq "$expected" ] || { echo 'Plugin dependency closure differs from lock' >&2; exit 1; }
