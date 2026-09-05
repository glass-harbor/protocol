#!/usr/bin/env bash
set -e
cd proto
buf dep update 2>/dev/null || buf mod update
buf generate --template buf.gen.gogo.yaml
cd ..
cp -r github.com/glass-harbor/protocol/* ./
rm -rf github.com
