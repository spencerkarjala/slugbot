package exec

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

const MAX_JOBS_IN_VIEW = 5

const (
	maxRows         = 3                // how many jobs to show in the view
	promptMaxLen    = 40               // max characters before we truncate
	promptCellWidth = promptMaxLen + 3 // total chars per row including '...'
)

type TaskQueueView struct {
	Queue     *TaskQueue
	Session   *discordgo.Session
	ChannelID string
	MessageID string
}

func NewTaskQueueView(q *TaskQueue, sess *discordgo.Session, channelID string) *TaskQueueView {
	return &TaskQueueView{Queue: q, Session: sess, ChannelID: channelID}
}

func (v *TaskQueueView) Refresh() error {
	// body := v.renderBody()

	// // if body is empty, then queue is empty, so just clean up and return
	// if body == "" {
	// 	if v.MessageID != "" {
	// 		_ = v.Session.ChannelMessageDelete(v.ChannelID, v.MessageID)
	// 		v.MessageID = ""
	// 	}
	// 	return nil
	// }

	// // fetch the most recent message in the channel
	// msgs, err := v.Session.ChannelMessages(v.ChannelID, 1, "", "", "")
	// if err != nil {
	// 	return fmt.Errorf("failed to fetch messages: %w", err)
	// }

	// // if the stored message is still the most recent one, then edit it
	// if len(msgs) > 0 && msgs[0].ID == v.MessageID {
	// 	_, err = v.Session.ChannelMessageEdit(v.ChannelID, v.MessageID, body)
	// 	return err
	// }

	// // otherwise, delete the old message and send a new one
	// if v.MessageID != "" {
	// 	_ = v.Session.ChannelMessageDelete(v.ChannelID, v.MessageID)
	// }
	// msg, err := v.Session.ChannelMessageSend(v.ChannelID, body)
	// if err != nil {
	// 	return fmt.Errorf("failed to send new queue view message: %w", err)
	// }
	// v.MessageID = msg.ID
	return nil
}

func formatCell(s string) string {
	rs := []rune(s)
	if len(rs) > promptCellWidth {
		return string(rs[:promptMaxLen]) + "..."
	}
	pad := promptCellWidth - len(rs)
	return s + strings.Repeat(" ", pad)
}

func (v *TaskQueueView) renderBody() string {
	v.Queue.mutex.Lock()
	defer v.Queue.mutex.Unlock()

	// jobs := v.Queue.queue
	// numJobs := len(jobs)
	// var lines []string

	// lines = append(lines,
	// 	"```",
	// 	fmt.Sprintf("╔═══╤═%s═╗", strings.Repeat("═", promptCellWidth)),
	// 	fmt.Sprintf("║ # │ %s ║", formatCell("Prompt")),
	// 	fmt.Sprintf("╟───┼─%s─╢", strings.Repeat("─", promptCellWidth)),
	// )

	// for i := 0; i < maxRows && i < numJobs; i++ {
	// 	prompt := formatCell(jobs[i].Prompt())
	// 	lines = append(lines, fmt.Sprintf("║ %d │ %s ║", i+1, prompt))
	// }

	// if numJobs > maxRows {
	// 	missing := fmt.Sprintf("... and %d more ...", numJobs-maxRows)
	// 	lines = append(lines, fmt.Sprintf("║   │ %s ║", formatCell(missing)))
	// } else {
	// 	lines = append(lines, fmt.Sprintf("║   │ %s ║", strings.Repeat(" ", promptCellWidth)))
	// }

	// lines = append(lines,
	// 	fmt.Sprintf("╚═══╧═%s═╝", strings.Repeat("═", promptCellWidth)),
	// 	"```",
	// )

	// return strings.Join(lines, "\n")
	return ""
}
