// Package api embeds the OpenAPI specification of the Acksy REST API.
package api

import _ "embed"

// OpenAPI is the OpenAPI 3.1 document (openapi.yaml).
//
//go:embed openapi.yaml
var OpenAPI []byte
