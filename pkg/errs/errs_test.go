package errs

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestErr(t *testing.T) {
	// the message also contains the full error.
	leaf := errors.Join(fmt.Errorf("Failed to create default drop route for ipv4: %w", unix.EINVAL), fmt.Errorf("Failed to create xfrm policy for ipv4: %w", unix.EINTR))
	// TODO: Managing doc links sounds like a nightmare.
	//	Need to check these?
	branch := WithHelp(fmt.Errorf("Enabling IPSec: %w", leaf), "Check error code, likely caused by host issues. Ensure that iproute2 >= v6.6 is installed. See: https://docs.cilium.io/en/latest/security/network/encryption-ipsec/ for details")
	err := WithHelp(fmt.Errorf("Failed to validate node using linux-datapath: %w", errors.Join(branch, fmt.Errorf("Enabling node encryption: %w", leaf))), "Failed node datapath validation: May be in degraded state")
	Print(err)
}

func TestErrSemantics(t *testing.T) {
	e1 := errors.New("1")
	err := WithHelp(fmt.Errorf("foo: %w", e1), "Some msg!")
	assert.ErrorIs(t, err, e1)
	err = fmt.Errorf("root: %w", errors.Join(fmt.Errorf("?"), WithHelp(fmt.Errorf("zzz: %w", e1), "???")))
	assert.ErrorIs(t, err, e1)
}
