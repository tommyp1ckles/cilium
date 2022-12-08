package dump

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {
	m := map[string]any{
		"Kind": "Dir",
		"Name": "foo",
		"Tasks": []map[string]any{
			{
				"Kind": "Exec",
				"Name": "bar",
				"Cmd":  "echo",
				"Args": []string{"hello"},
			},
		},
	}
	tf := &TaskFactory{}
	root, err := tf.decode(m)
	assert.NoError(t, err)
	fmt.Printf("%T\n", root)
	fmt.Println(root)
}

func TestEncode(t *testing.T) {
	root := &Dir{
		base: base{
			Kind: "Dir",
			Name: "d0",
		},
		Tasks: []Task{
			&Exec{
				base: base{
					Kind: "Exec",
					Name: "e0",
				},
				Cmd:  "ls",
				Args: []string{"-l", "/etc/"},
			},
			&Exec{
				base: base{
					Kind: "Exec",
					Name: "e1",
				},
				Cmd:  "ls",
				Args: []string{"-l", "/var/"},
			},
		},
	}
	d, err := json.MarshalIndent(root, "", "	")
	assert.NoError(t, err)
	fmt.Println(string(d))

	m := map[string]any{}
	assert.NoError(t, json.Unmarshal(d, &m))

	tf := &TaskFactory{}
	fmt.Println(m)
	rootTask, err := tf.decode(m)
	assert.NoError(t, err)
	fmt.Println(rootTask)
}
