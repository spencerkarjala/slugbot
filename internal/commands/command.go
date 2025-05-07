package commands

import (
	"github.com/bwmarrin/discordgo"
)

type Command struct {
	Session *discordgo.Session
	Message *discordgo.MessageCreate
}

func (c *Command) SetContext(s *discordgo.Session, m *discordgo.MessageCreate) {
	c.Session = s
	c.Message = m
}

func (c *Command) HandleError(err error) {
	c.Session.ChannelMessageSend(c.Message.ChannelID, "Error occurred while processing: "+err.Error())
}

type CommandHandler interface {
	SetContext(s *discordgo.Session, m *discordgo.MessageCreate)
	Usage() string
	Validate() error
	Apply() error
	HandleError(error)
}
