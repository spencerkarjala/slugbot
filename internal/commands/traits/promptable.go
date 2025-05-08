package traits

// Promptable represents commands that carry user-entered text.
type Promptable interface {
	Prompt() string
	SetPrompt(text string)
}

// PromptHandler is a helper you can embed to implement Promptable.
type PromptHandler struct {
	prompt string
}

func (h *PromptHandler) Prompt() string {
	return h.prompt
}

func (h *PromptHandler) SetPrompt(text string) {
	h.prompt = text
}
