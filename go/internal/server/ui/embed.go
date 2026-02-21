package ui

import "embed"

//go:embed templates/* static/*
var FS embed.FS // FS is the embedded filesystem for UI assets
