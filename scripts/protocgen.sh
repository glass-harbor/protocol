#!/usr/bin/env bash
set -e
cd proto
# buf.lock is committed; only resolve deps when it is missing so proto-check is offline and deterministic.
if [ ! -f buf.lock ]; then
  buf dep update 2>/dev/null || buf mod update
fi
buf generate --template buf.gen.gogo.yaml
cd ..
cp -r github.com/glass-harbor/protocol/* ./
rm -rf github.com
