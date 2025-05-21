package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/zalando/go-keyring"

	"slugbot/internal/commands"
	"slugbot/internal/commands/audio"
	"slugbot/internal/commands/image"
	"slugbot/internal/exec"
	"slugbot/internal/io/slog"
)

// Create mapping from command strings to factory functions for each command type
var commandHandlers = map[string]func() commands.CommandHandler{
	"arc":       func() commands.CommandHandler { return &image.ArcDistortCommand{} },
	"barrel":    func() commands.CommandHandler { return &image.BarrelDistortCommand{} },
	"ibarrel":   func() commands.CommandHandler { return &image.InverseBarrelDistortCommand{} },
	"polar":     func() commands.CommandHandler { return &image.PolarDistortCommand{} },
	"ipolar":    func() commands.CommandHandler { return &image.InversePolarDistortCommand{} },
	".saudio":   func() commands.CommandHandler { return &audio.StableAudioCommand{} },
	"```saudio": func() commands.CommandHandler { return &audio.StableAudioWithConfigCommand{} },
	"```toml":   func() commands.CommandHandler { return &audio.StableAudioWithConfigCommand{} },
	".slimit":   func() commands.CommandHandler { return &audio.LimitCommand{} },
}

var audioQueue = *exec.NewTaskQueue()
var audioQueueView *exec.TaskQueueView

func UpdateQueueViewCallback(view *exec.TaskQueueView) {
	if view == nil {
		slog.Error("received nil view in UpdateQueueViewCallback")
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := view.Refresh(); err != nil {
			slog.Error("failed to refresh view in channel %s; %v\r\n", view.ChannelID, err)
		}
	}
}

func getCommandList() string {
	var keys []string
	for key := range commandHandlers {
		keys = append(keys, "`"+key+"`")
	}
	return strings.Join(keys, ", ")
}

func messageCreateHandler(session *discordgo.Session, message *discordgo.MessageCreate) {
	if message == nil || message.Author == nil || message.Author.Bot {
		return
	}

	content := strings.TrimSpace(message.Content)
	if len(content) < 1 {
		return
	}

	parts := strings.Fields(message.Content)

	if parts[0] != ".sim" && parts[0] != ".saudio" && parts[0] != ".saudiosm" && parts[0] != ".slimit" && parts[0] != "```saudio" && parts[0] != "```toml" {
		return
	}

	if parts[0] == ".imagine" {
		return
	}

	if parts[0] == ".saudio" || parts[0] == ".saudiosm" {
		commandConstructor, ok := commandHandlers[".saudio"]
		if !ok {
			session.ChannelMessageSend(message.ChannelID, "Error occured while processing .saudio prompt")
			return
		}
		command := commandConstructor()
		command.SetContext(session, message)

		// need to validate input before we can save the prompt
		if err := command.Validate(); err != nil {
			session.ChannelMessageSend(message.ChannelID, command.Usage())
			slog.Error("couldn't validate Stable Audio command: %v", err)
			return
		}

		// command should be an audio-generation command, so leave if it's not Promptable
		stableAudioCommand, ok := command.(*audio.StableAudioCommand)
		if !ok {
			slog.Fatal("somehow created a non-Stable-Audio command from .saudio prompt")
			return
		}

		// finally, set the prompt
		parts = append(parts, "--small")
		stableAudioCommand.SetPrompt(strings.Join(parts[1:], " "))

		if audioQueueView == nil {
			audioQueueView := *exec.NewTaskQueueView(&audioQueue, session, message.ChannelID)
			go UpdateQueueViewCallback(&audioQueueView)
		}

		audioQueue.Enqueue(stableAudioCommand)
		return
	}

	if parts[0] == "```saudio" || parts[0] == "```toml" {
		commandConstructor, ok := commandHandlers["```saudio"]
		if !ok {
			session.ChannelMessageSend(message.ChannelID, "Error occured while processing .saudio prompt")
			return
		}
		command := commandConstructor()
		command.SetContext(session, message)

		if audioQueueView == nil {
			audioQueueView := *exec.NewTaskQueueView(&audioQueue, session, message.ChannelID)
			go UpdateQueueViewCallback(&audioQueueView)
		}

		stableAudioCommand, ok := command.(*audio.StableAudioWithConfigCommand)
		if !ok {
			slog.Fatal("somehow created a non-Stable-Audio command from ```saudio prompt")
			return
		}

		audioQueue.Enqueue(stableAudioCommand)
		return
	}

	if parts[0] == ".slimit" {
		commandConstructor, ok := commandHandlers[".slimit"]
		if !ok {
			session.ChannelMessageSend(message.ChannelID, "Error occurred while processing .slimit prompt")
			return
		}
		command := commandConstructor()
		command.SetContext(session, message)

		slimitCommand, ok := command.(*audio.LimitCommand)
		if !ok || slimitCommand == nil {
			slog.Fatal("somehow created a non-limit command from .slimit prompt")
			return
		}

		slog.Info("applying .slimit command...")
		slimitCommand.Apply()
		return
	}

	if len(parts) < 2 {
		session.ChannelMessageSend(message.ChannelID, "Usage: .sim <word>")
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

func loadDiscordToken() (string, error) {
	token, err := keyring.Get("slugbot-production", "token")
	if err == keyring.ErrNotFound {
		fmt.Print("Enter your Discord API token:")
		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		input = strings.TrimSpace(input)
		if err := keyring.Set("slugbot-production", "token", input); err != nil {
			return "", fmt.Errorf("couldn't save token: %w", err)
		}
		return input, nil
	} else if err != nil {
		return "", fmt.Errorf("couldn't retrieve token: %w", err)
	}
	return token, nil
}

func main() {
	token, err := loadDiscordToken()
	if err != nil {
		slog.Error("error loading Discord token, ", err)
	}
	slog.SetLevel(slog.LevelTrace)

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		slog.Error("error creating Discord session,", err)
		return
	}

	dg.AddHandler(messageCreateHandler)

	err = dg.Open()
	if err != nil {
		slog.Error("error opening connection,", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	dg.Close()
}
