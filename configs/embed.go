// Package configs embeds the default configuration for standalone binaries.
package configs

import _ "embed"

//go:embed config.yaml
var Default []byte
