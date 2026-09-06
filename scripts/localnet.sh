#!/usr/bin/env bash
# Single-validator localnet (SPEC §9). Usage: scripts/localnet.sh [start|init|reset]
set -euo pipefail

HOME_DIR="${HARBORD_HOME:-$HOME/.harbord-local}"
CHAIN_ID="glassharbor-local-1"
BIN="${HARBORD_BIN:-$(command -v harbord || echo "$(cd "$(dirname "$0")/.." && pwd)/build/harbord")}"
KR=(--keyring-backend test --home "$HOME_DIR")

init() {
  if [ -f "$HOME_DIR/config/genesis.json" ]; then
    echo "localnet already initialised at $HOME_DIR"
    "$BIN" config set app api.swagger true --home "$HOME_DIR" --skip-validate
    return
  fi
  "$BIN" init local --chain-id "$CHAIN_ID" --default-denom uglass --home "$HOME_DIR" >/dev/null 2>&1
  for k in validator treasury alice bob; do
    "$BIN" keys add "$k" "${KR[@]}" --output json >/dev/null 2>&1
    "$BIN" genesis add-genesis-account "$("$BIN" keys show "$k" -a "${KR[@]}")" 1000000000000uglass --home "$HOME_DIR"
  done
  local gen="$HOME_DIR/config/genesis.json" treasury
  treasury="$("$BIN" keys show treasury -a "${KR[@]}")"
  jq --arg t "$treasury" '
    .app_state.registry.params.treasury_address = $t
    | .app_state.registry.params.voting_period = "30s"
    | .app_state.gov.params.voting_period = "60s"
    | .app_state.gov.params.expedited_voting_period = "30s"
    | .app_state.staking.params.unbonding_time = "60s"
  ' "$gen" > "$gen.tmp" && mv "$gen.tmp" "$gen"
  "$BIN" genesis gentx validator 100000000000uglass --chain-id "$CHAIN_ID" "${KR[@]}" >/dev/null 2>&1
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null 2>&1
  "$BIN" genesis validate --home "$HOME_DIR"
  "$BIN" config set app minimum-gas-prices 0.001uglass --home "$HOME_DIR" --skip-validate
  "$BIN" config set app api.enable true --home "$HOME_DIR" --skip-validate
  "$BIN" config set app api.swagger true --home "$HOME_DIR" --skip-validate
  "$BIN" config set config consensus.timeout_commit 1s --home "$HOME_DIR" --skip-validate
  echo "localnet initialised at $HOME_DIR (treasury=$treasury)"
}

case "${1:-start}" in
  reset) rm -rf "$HOME_DIR"; echo "removed $HOME_DIR" ;;
  init) init ;;
  start) init; exec "$BIN" start --home "$HOME_DIR" ;;
  *) echo "usage: $0 [start|init|reset]" >&2; exit 2 ;;
esac
