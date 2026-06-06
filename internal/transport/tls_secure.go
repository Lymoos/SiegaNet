//go:build !phase0_insecure

// This file is compiled for every build that does NOT set the phase0_insecure
// tag. The insecure Phase 0 helpers are stubbed out so a normal (Phase 1+)
// build physically cannot create a self-signed/verification-skipping config:
// the functions exist for type compatibility but always error.
package transport

import (
	"crypto/tls"
	"errors"
)

// InsecureBuild reports whether this binary was built with the phase0_insecure tag.
const InsecureBuild = false

var errInsecureDisabled = errors.New(
	"transport: insecure Phase 0 TLS is disabled; rebuild with -tags phase0_insecure to use it")

// SelfSignedTLS is disabled without the phase0_insecure build tag.
func SelfSignedTLS(string) (*tls.Config, error) { return nil, errInsecureDisabled }

// InsecureClientTLS is disabled without the phase0_insecure build tag.
func InsecureClientTLS(string) (*tls.Config, error) { return nil, errInsecureDisabled }
