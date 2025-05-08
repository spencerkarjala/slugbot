package traits

// Promptable is a helper you can embed to implement PromptHandler.
type Promptable struct {
	prompt string
}

// PromptHandler represents commands that carry user-entered text.
type PromptHandler interface {
	Prompt() string
	SetPrompt(text string)
}

func (h *Promptable) Prompt() string {
	return h.prompt
}

func (h *Promptable) SetPrompt(text string) {
	h.prompt = text
}
