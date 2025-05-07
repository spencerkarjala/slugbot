package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/bwmarrin/discordgo"

	"test-discord-bot/commands"
	"test-discord-bot/commands/audio"
	"test-discord-bot/commands/image"
)

// Create mapping from command strings to factory functions for each command type
var commandHandlers = map[string]func() commands.CommandHandler{
	"arc":     func() commands.CommandHandler { return &image.ArcDistortCommand{} },
	"barrel":  func() commands.CommandHandler { return &image.BarrelDistortCommand{} },
	"ibarrel": func() commands.CommandHandler { return &image.InverseBarrelDistortCommand{} },
	"polar":   func() commands.CommandHandler { return &image.PolarDistortCommand{} },
	"ipolar":  func() commands.CommandHandler { return &image.InversePolarDistortCommand{} },
	".saudio": func() commands.CommandHandler { return &audio.StableAudioCommand{} },
}

func getCommandList() string {
	var keys []string
	for key := range commandHandlers {
		keys = append(keys, "`"+key+"`")
	}
	return strings.Join(keys, ", ")
}

func messageCreateHandler(session *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Author.Bot {
		return
	}
	if !strings.HasPrefix(message.Content, ".im") && !strings.HasPrefix(message.Content, ".saudio") {
		return
	}

	parts := strings.Fields(message.Content)

	if parts[0] == ".saudio" {
		commandConstructor, ok := commandHandlers[".saudio"]
		if !ok {
			session.ChannelMessageSend(message.ChannelID, "Error occured while processing .saudio prompt")
			return
		}
		command := commandConstructor()
		command.SetContext(session, message)
		if err := command.Apply(); err != nil {
			session.ChannelMessageSend(message.ChannelID, "Error occurred while processing: "+err.Error())
			return
		}
		return
	}

	if len(parts) < 2 {
		session.ChannelMessageSend(message.ChannelID, "Usage: .im <word>")
		return
	}

	commandString := parts[1]
	commandConstructor, ok := commandHandlers[commandString]
	if !ok {
		session.ChannelMessageSend(message.ChannelID, "Received unknown command '`"+commandString+"`'; must be one of '"+getCommandList()+"'")
		return
	}

	command := commandConstructor()
	command.SetContext(session, message)
	if err := command.Apply(); err != nil {
		session.ChannelMessageSend(message.ChannelID, "Error occurred while processing: "+err.Error())
	}
}

func main() {
	token := ""

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	dg.AddHandler(messageCreateHandler)

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	dg.Close()
}
