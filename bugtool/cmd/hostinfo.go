package cmd

import (
	"fmt"
	"os"
	"syscall"
)

type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"sysname"`
}

func unameFieldToString(a [65]int8) (string, error) {
	bs := make([]byte, len(a))
	for i, b := range a {
		if b < 0 {
			return "", fmt.Errorf("should not be negative")
		}
		bs[i] = uint8(b)
	}
	return string(bs), nil
}

func (h *HostInfo) populate() error {
	hn, err := os.Hostname()
	if err != nil {
		return err
	}
	h.Hostname = hn

	buf := &syscall.Utsname{}
	if err := syscall.Uname(buf); err != nil {
		return err
	}

	h.OS, err = unameFieldToString(buf.Sysname)
	if err := syscall.Uname(buf); err != nil {
		return err
	}
	return nil
}
