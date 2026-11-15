// Package demodata содержит встроенные YAML-данные для воспроизводимого демо.
package demodata

import "embed"

//go:embed *.yaml
var Files embed.FS
