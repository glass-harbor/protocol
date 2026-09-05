#!/usr/bin/env bash
# End-to-end smoke test against a running localnet (SPEC §9). Exit 0 iff the blue-check flow works.
set -euo pipefail

HOME_DIR="${HARBORD_HOME:-$HOME/.harbord-local}"
CHAIN_ID="glassharbor-local-1"
BIN="${HARBORD_BIN:-$(cd "$(dirname "$0")/.." && pwd)/build/harbord}"
KR=(--keyring-backend test --home "$HOME_DIR")
TXF=(--chain-id "$CHAIN_ID" --yes --gas 400000 --fees 1000uglass --output json "${KR[@]}")
MAGNET="magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=smoke.zip"
SHA="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

addr() { "$BIN" keys show "$1" -a "${KR[@]}"; }
bal() { "$BIN" query bank balance "$(addr "$1")" uglass --output json --home "$HOME_DIR" | jq -r '.balance.amount'; }
tx() {
  local hash
  hash=$("$BIN" tx registry "$@" "${TXF[@]}" | jq -r '.txhash')
  "$BIN" query wait-tx "$hash" --output json --home "$HOME_DIR" | jq -e '.code == 0' >/dev/null \
    || { echo "tx failed: $*" >&2; "$BIN" query tx "$hash" --output json --home "$HOME_DIR" | jq -r '.raw_log' >&2; exit 1; }
}
assert_eq() { [ "$1" = "$2" ] || { echo "ASSERT FAILED ($3): got $1 want $2" >&2; exit 1; }; }

echo "waiting for node..."
for _ in $(seq 1 60); do
  if "$BIN" status --home "$HOME_DIR" 2>/dev/null | jq -e '.sync_info.latest_block_height | tonumber > 1' >/dev/null 2>&1; then break; fi
  sleep 1
done

t0=$(bal treasury); v0=$(bal validator)

tx create-app --title "Smoke App" --description "smoke" --category utilities --from alice
tx publish-version 1 1.0.0 "$MAGNET" "$SHA" 1234 --from alice
tx request-blue-check 1 1.0.0 --escrow 1000000uglass --from alice
# AutoCLI accepts the enum suffix ("yes"); fall back to the full enum name if this client build does not.
if "$BIN" tx registry vote 1 yes --from validator "${TXF[@]}" >/dev/null 2>&1; then
  sleep 3
else
  tx vote 1 VOTE_OPTION_YES --from validator
fi

echo "waiting for the 30s voting period..."
sleep 40

assert_eq "$("$BIN" query registry app 1 --output json --home "$HOME_DIR" | jq -r '.verified')" "true" "app verified"
assert_eq "$("$BIN" query registry request 1 --output json --home "$HOME_DIR" | jq -r '.request.status')" "REQUEST_STATUS_PASSED" "request passed"
assert_eq "$("$BIN" query registry versions 1 --output json --home "$HOME_DIR" | jq -r '.versions[0].blue_check')" "true" "version blue check"
assert_eq "$(bal treasury)" "$((t0 + 1000000 + 500000 + 100000))" "treasury received fee cuts and escrow cut"
assert_eq "$(bal validator)" "$((v0 + 900000 - 1000))" "validator received escrow share minus vote fee"
echo "SMOKE OK"
