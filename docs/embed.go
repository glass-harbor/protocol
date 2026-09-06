// Package docs embeds the generated OpenAPI spec (make proto-gen).
package docs

import _ "embed"

// OpenAPI is the registry module's OpenAPI 2.0 spec, generated from query.proto.
//
//go:embed static/openapi.swagger.yaml
var OpenAPI []byte
