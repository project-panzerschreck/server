package llama

type Llama struct {
	Command []string
}

// checkValidity checks that the Ramalama configuration is valid.
// A valid config must have a non-empty Command slice.
func (r Llama) checkValidity() error {
	if len(r.Command) == 0 {
		return ErrEmptyCommand{}
	}
	return nil
}

type Model struct {
	Name     string
	Modified string
	Size     int
}

func (c Llama) GetModels() ([]Model, error) {
	return []Model{
		{
			Name: "hf:unsloth/Qwen3-0.6B-GGUF:UD-Q4_K_XL",
		},
	}, nil
}
