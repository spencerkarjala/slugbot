package exec

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const MAX_JOBS_IN_VIEW = 5

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
	body := v.renderBody()

	// if body is empty, then queue is empty, so just clean up and return
	if body == "" {
		if v.MessageID != "" {
			_ = v.Session.ChannelMessageDelete(v.ChannelID, v.MessageID)
			v.MessageID = ""
		}
		return nil
	}

	// fetch the most recent message in the channel
	msgs, err := v.Session.ChannelMessages(v.ChannelID, 1, "", "", "")
	if err != nil {
		return fmt.Errorf("failed to fetch messages: %w", err)
	}

	// if the stored message is still the most recent one, then edit it
	if len(msgs) > 0 && msgs[0].ID == v.MessageID {
		_, err = v.Session.ChannelMessageEdit(v.ChannelID, v.MessageID, body)
		return err
	}

	// otherwise, delete the old message and send a new one
	if v.MessageID != "" {
		_ = v.Session.ChannelMessageDelete(v.ChannelID, v.MessageID)
	}
	msg, err := v.Session.ChannelMessageSend(v.ChannelID, body)
	if err != nil {
		return fmt.Errorf("failed to send new queue view message: %w", err)
	}
	v.MessageID = msg.ID
	return nil
}

func (v *TaskQueueView) renderBody() string {
	v.Queue.mutex.Lock()
	defer v.Queue.mutex.Unlock()

	numJobs := len(v.Queue.queue)
	if numJobs < 1 {
		return ""
	}

	numJobsToDisplay := min(numJobs, MAX_JOBS_IN_VIEW)

	var lines []string
	for i := range numJobsToDisplay {
		task := v.Queue.queue[i]
		lines = append(lines, fmt.Sprintf("%d) %T", i+1, task))
	}

	if numJobs > MAX_JOBS_IN_VIEW {
		numRemainingJobs := numJobs - MAX_JOBS_IN_VIEW
		lines = append(lines, fmt.Sprintf("...and %d more...", numRemainingJobs))
	}

	return strings.Join(lines, "\n")
}
