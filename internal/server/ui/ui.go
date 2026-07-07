package ui

import "embed"

// Files contains the embedded frontend static assets.
//
//go:embed dist/*
var Files embed.FS
