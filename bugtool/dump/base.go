package dump

import "fmt"

type Base struct {
	Name string `json:"Name",mapstructure:"Name"`
	Kind string `json:"Kind",mapstructure:"Kind"`
}

func (b Base) validate() error {
	if b.Kind == "" {
		return fmt.Errorf("task kind cannot be empty")
	}
	switch b.Kind {
	case "Dir", "Exec", "File", "Request":
		return nil
	default:
		return fmt.Errorf("unknown task kind %q", b.Kind)
	}
}
