package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildContentSecurityPolicyIncludesIrmaOrigin(t *testing.T) {
	csp := buildContentSecurityPolicy("https://is.staging.yivi.app")
	require.Contains(t, csp, "default-src 'self'")
	require.Contains(t, csp, "connect-src 'self' https://is.staging.yivi.app")
	require.NotContains(t, csp, "unsafe-inline")
	require.NotContains(t, csp, "unsafe-eval")
}

func TestBuildContentSecurityPolicyFallsBackOnInvalidUrl(t *testing.T) {
	csp := buildContentSecurityPolicy("")
	require.Contains(t, csp, "connect-src 'self'")
}
