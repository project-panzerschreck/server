package llama

type ErrEmptyCommand struct{}

// Error implements error.
func (e ErrEmptyCommand) Error() string {
	return "llama command should not be empty"
}

var _ error = ErrEmptyCommand{}
