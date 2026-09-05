package app

import "encoding/json"

// GenesisState is the app-level genesis: module name -> raw JSON.
type GenesisState map[string]json.RawMessage
