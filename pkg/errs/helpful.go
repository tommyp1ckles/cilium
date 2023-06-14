package errs

import (
	"errors"
	"fmt"
	"strings"
)

// This is some example code for how we might approach dealing with errors
// emitted by Cilium reconciliation code.
//
// That is, code that performs procedures that are intended to reconcile "realized"
// and actual system state.
//
// Currently, our error handling is inconsistent, this seeks to improve:
// * Provide a way of dealing with complex, heirachical, error traces in a easy, and un-univasive way.
// * Improve error reporting such that it is:
//		* More meaningful for engineers/users by providing better context on what/where the failure occured.
//      * Provide more end-user facing error messages that supply potentially additional helpful information, links, data, etc.
//
// This library provides some functionality built on top of existing Go error handling patterns, as well as the standard "errors" library
// 	to help developers write/report errors.
//
//
// Example Error (Print):
//
// Failed to validate node using linux-datapath
//   Enabling IPSec: Check error code, likely caused by host issues. Ensure that iproute2 >= v6.6 is installed. See: https://docs.cilium.io/en/latest/security/network/encryption-ipsec/ for details
//     Failed to create default drop route for ipv4
//       invalid argument
//     Failed to create xfrm policy for ipv4
//       interrupted system call
//   Enabling node encryption
//     Failed to create default drop route for ipv4
//       invalid argument
//     Failed to create xfrm policy for ipv4
//       interrupted system call
//
// Visualizing Errors:
//               ┌──────────┐
//               │Root Err  │ <- High level error, something like
//               │          │     Ex. "Failed to enable IPSec: ..."
//               │          │
//               │          │
//               └─────┬────┘
//                     │
//       ┌─────────────┼──────────────┐
//       │             │              │
// ┌─────┴────┐   ┌────┴─────┐  ┌─────┴────┐
// │Sub Err A │   │ Sub Err B│  │ Sub Err C│ <- Sub errors, where reconcile procedure failed.
// │          │   │          │  │          │       Ex. 1."Failed to apply xfrm policy: ..."
// │          │   │          │  │          │           2."Failed to replace default drop route for ipv6: ..."
// │          │   │          │  │          │
// └─────┬────┘   └──────┬───┘  └──────────┘
//       │               │
//       │               │
//       │               │
// ┌─────┴────┐     ┌────┴─────┐
// │Leaf Err 1│     │Leaf Err 2│
// │          │     │          │ <- Intermediate errors, from utility libraries:
// │          │     │          │     Ex."Netlink apply XFRM policy (...): ..."
// │          │     │          │
// └──────────┘     └─────┬────┘
//                        │
//                        │
//                   ┌────┴─────┐
//                   │          │
//                   │          │ <- Actual "leaf" errors, likely things like unix Syscall errs etc...
//                   │          │     Ex."EINVAL: invalid argument
//                   │          │
//                   └──────────┘

// ciliumError implements an error, that adds additional help information.
// When unrwapped, this behaves like a normal error, the only way to access
// the help message is by doing (checked!) dynamic type assertions to see if
// an error has some Cilium specific help info.
//
// TODO: Think of a better name?
type ciliumError struct {
	helpMessage string
	err         error
}

func WithHelp(err error, help string) error {
	return &ciliumError{helpMessage: help, err: err}
}

func getHelpMessage(err error) (*string, error) {
	if ce, ok := err.(*ciliumError); ok {
		return &ce.helpMessage, ce.err
	}
	return nil, err
}

func (c *ciliumError) Error() string {
	return c.err.Error()
}

func (c *ciliumError) Unwrap() error {
	return c.err
}

func Unwrap(err error) []error {
	if errs, ok := err.(interface{ Unwrap() []error }); ok {
		return errs.Unwrap()
	} else {
		// In the case like fmt.Errorf('root: %w', errs) the first
		// unwrap is just the flat error of errs, because we're interested
		// in the "children" joined errs we attempt another unwrap to see
		// if there is a list of underlying errors to this value.
		err = errors.Unwrap(err)
		var errs []error
		if es, ok := err.(interface{ Unwrap() []error }); ok {
			errs = es.Unwrap()
		} else {
			errs = append(errs, err)
		}
		return errs
	}
}

func Message(err error) string {
	if child := errors.Unwrap(err); child == nil {
		// If we're at a leaf error, just return the whole error message.
		return err.Error()
	} else {
		s := strings.ReplaceAll(err.Error(), child.Error(), "")
		s = strings.TrimRight(s, ": ")
		return s
	}
}

func fmtMsg(s string, indent int) string {
	return strings.ReplaceAll(s, "\n", "\n"+strings.Repeat(".", indent))
}

func Print(err error) {
	printErr(0, err)
}

func printErr(indent int, err error) {
	if err == nil {
		return
	}
	if help, err2 := getHelpMessage(err); help != nil {
		fmt.Printf("%s%s: %s\n", strings.Repeat(" ", indent), fmtMsg(Message(err2), indent), *help)
		err = err2
	} else {
		fmt.Printf("%s%s\n", strings.Repeat(" ", indent), fmtMsg(Message(err), indent))
	}
	for _, err := range Unwrap(err) {
		printErr(indent+2, err)
	}
}
