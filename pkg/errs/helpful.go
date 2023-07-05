package errs

import (
	"errors"
	"fmt"
	"strings"

	eventspb "github.com/cilium/cilium/api/v1/events"
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

// helpError implements an error, that adds additional help information.
// When unrwapped, this behaves like a normal error, the only way to access
// the help message is by doing (checked!) dynamic type assertions to see if
// an error has some Cilium specific help info.
//
// TODO: Think of a better name?
type helpError struct {
	helpMessage string
	err         error
}

func WithHelp(err error, help string) error {
	return &helpError{helpMessage: help, err: err}
}

func getHelpMessage(err error) (*string, error) {
	if ce, ok := err.(*helpError); ok {
		return &ce.helpMessage, ce.err
	}
	return nil, err
}

func (c *helpError) Error() string {
	return c.err.Error()
}

func (c *helpError) Unwrap() error {
	// Try to do underlying unwrap?
	if i, ok := c.err.(interface{ Unwrap() error }); ok {
		return i.Unwrap()
	}
	return c.err
}

func Unwrap(err error) []error {
	_, err = getHelpMessage(err)
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

// Message extracts the wrap context message from an error, without the underlying
// err.
func Message(err error) string {
	if child := errors.Unwrap(err); child == nil {
		// If we're at a leaf error, just return the whole error message.
		return err.Error()
	} else {
		// If the error doesn't follow the pattern of msg: err, then we add
		// a placeholder instead.
		if !strings.HasSuffix(err.Error(), child.Error()) {
			return strings.ReplaceAll(err.Error(), child.Error(), "<error>")
		}

		// If the error does not follow the suffix: err format.
		s := strings.Replace(err.Error(), child.Error(), "", 1)
		s = strings.TrimRight(s, ": ")
		return s
	}
}

func fmtMsg(s string, indent int) string {
	return strings.ReplaceAll(s, "\n", "\n"+strings.Repeat(".", indent))
}

type traceNode struct {
	msg      string      `json:"msg"`
	children []traceNode `json:"children"`
}

func Tree(err error) traceNode {
	r := traceNode{msg: Message(err)}
	for _, err := range Unwrap(err) {
		if err == nil {
			continue
		}
		r.children = append(r.children, Tree(err))
	}
	return r
}

func Serialize(err error) *eventspb.Error {
	if err == nil {
		return nil
	}
	var helpMsg string
	if help, err2 := getHelpMessage(err); help != nil {
		helpMsg = *help
		err = err2
	}

	var children []*eventspb.Error
	for _, err := range Unwrap(err) {
		fmt.Println("=>", err)
		if c := Serialize(err); c != nil {
			children = append(children, c)
		}
	}
	m := Message(err)
	return &eventspb.Error{
		Message: m,
		Help:    []string{helpMsg},
		Wrapped: children,
	}
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

func ToStatusMessage(err error) string {
	return appendMessage("", 0, err)
}

func appendMessage(buf string, indent int, err error) string {
	if err == nil {
		return buf
	}
	if help, err2 := getHelpMessage(err); help != nil {
		l := fmt.Sprintf("%s%s: %s\n", strings.Repeat(" ", indent), fmtMsg(Message(err2), indent), *help)
		buf += l
		err = err2
	} else {
		l := fmt.Sprintf("%s%s\n", strings.Repeat(" ", indent), fmtMsg(Message(err), indent))
		buf += l
	}
	for _, err := range Unwrap(err) {
		buf = appendMessage(buf, indent+2, err)
	}
	return buf
}
