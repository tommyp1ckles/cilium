package main

import (
	"fmt"

	"github.com/cilium/cilium/pkg/maps/ctmap"
	"github.com/cilium/cilium/pkg/tuple"
)

func main() {
	fmt.Println("vim-go")
	m := ctmap.GlobalMaps(true, false)[0]
	if err := m.OpenOrCreate(); err != nil {
		panic(err)
	}
	k := &ctmap.CtKey4{
		TupleKey4: tuple.TupleKey4{
			DestPort: 0xbeef,
		},
	}
	v := &ctmap.CtEntry{}
	if err := m.Update(k, v); err != nil {
		panic(err)
	}
}
