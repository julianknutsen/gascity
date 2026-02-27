package main

import "embed"

//go:embed prompts/*.md
var defaultPrompts embed.FS

//go:embed formulas/*.toml
var defaultFormulas embed.FS

//go:embed all:system_formulas
var systemFormulasFS embed.FS
