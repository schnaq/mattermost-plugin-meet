package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGoogleAccounts(t *testing.T) {
	lookup := func(u string) (string, error) {
		if u == "alice" {
			return "uid1", nil
		}
		return "", errors.New("unknown")
	}
	m, errs := parseGoogleAccounts(" alice = alice@example.com \n# x\n\nbad\nghost=g@x.de\nbob=nomail", lookup)
	require.Equal(t, map[string]string{"uid1": "alice@example.com"}, m)
	require.Len(t, errs, 3)
}
