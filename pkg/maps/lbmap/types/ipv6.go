package types

import (
	"github.com/cilium/cilium/pkg/loadbalancer"
	"github.com/cilium/cilium/pkg/types"
	"github.com/cilium/cilium/pkg/u8proto"
)

type Service6Key struct {
	Address     types.IPv6 `align:"address"`
	Port        uint16     `align:"dport"`
	BackendSlot uint16     `align:"backend_slot"`
	Proto       uint8      `align:"proto"`
	Scope       uint8      `align:"scope"`
	Pad         Pad2uint8  `align:"pad"`
}

type Service6Value struct {
	BackendID uint32    `align:"backend_id"`
	Count     uint16    `align:"count"`
	RevNat    uint16    `align:"rev_nat_index"`
	Flags     uint8     `align:"flags"`
	Flags2    uint8     `align:"flags2"`
	Pad       Pad2uint8 `align:"pad"`
}

type Backend6Value struct {
	Address types.IPv6      `align:"address"`
	Port    uint16          `align:"port"`
	Proto   u8proto.U8proto `align:"proto"`
	Flags   uint8           `align:"flags"`
}

type RevNat6Key struct {
	Key uint16
}

type RevNat6Value struct {
	Address types.IPv6 `align:"address"`
	Port    uint16     `align:"port"`
}

type Backend6KeyV2 struct {
	ID loadbalancer.BackendID
}

type Backend6Key struct {
	ID uint16
}

type SockRevNat6Key struct {
	Cookie  uint64     `align:"cookie"`
	Address types.IPv6 `align:"address"`
	Port    int16      `align:"port"`
	Pad     int16      `align:"pad"`
}

type SockRevNat6Value struct {
	Address     types.IPv6 `align:"address"`
	Port        int16      `align:"port"`
	RevNatIndex uint16     `align:"rev_nat_index"`
}
