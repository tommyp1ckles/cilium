package dump

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncode(t *testing.T) {
	assert := assert.New(t)
	root := &Dir{
		Base: Base{
			Kind: "Dir",
			Name: "d0",
		},
		Tasks: []Task{
			&Exec{
				Base: Base{
					Kind: "Exec",
					Name: "e0",
				},
				Cmd:  "ls",
				Args: []string{"/etc/"},
			},
			&Exec{
				Base: Base{
					Kind: "Exec",
					Name: "e1",
				},
				Cmd:  "bpftool",
				Args: []string{"net", "show"},
			},
			&Dir{Base: Base{Kind: KindDir, Name: "z"}},
			&Request{Base: Base{Kind: KindRequest, Name: "z"}},
		},
	}
	d, err := json.MarshalIndent(root, "", "	")
	assert.NoError(err)
	fmt.Println(string(d))

	m := map[string]any{}
	assert.NoError(json.Unmarshal(d, &m))

	tf := &TaskDecoder{}
	fmt.Println(m)
	rootTask, err := tf.decode(m)
	assert.NoError(err)
	assert.EqualValues(root, rootTask)
}
