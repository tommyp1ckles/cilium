package ipcache

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSON(t *testing.T) {
	assert := assert.New(t)
	k := &Key{
		IP: [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	}
	d, err := json.MarshalIndent(k, "", "    ")
	assert.NoError(err)
	fmt.Println(string(d))
}
