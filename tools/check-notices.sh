#!/usr/bin/env sh
set -e

for dep in $(go list -m all | tail -n +2 | cut -d' ' -f1)
do
    grep --fixed-strings -q "$dep" NOTICES.md \
        || (echo "missing $dep from NOTICES.md" && exit 1)
done
