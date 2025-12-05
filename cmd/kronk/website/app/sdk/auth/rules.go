package auth

import (
	_ "embed"
)

// These are the current set of rules we have for auth.
const (
	RuleAuthenticate = "auth"
)

// Package name of our rego code.
const (
	opaPackage string = "ardan.rego"
)

// Core OPA policies.
var (
	//go:embed rego/authentication.rego
	regoAuthentication string
)
