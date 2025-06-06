package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"slices"
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

// Top-level commands such as `.saudio` or `.slimit`
var topCommandHandlers = map[string]func(*discordgo.Session, *discordgo.MessageCreate) error{
	".sim":      handleDotSim,
	".saudio":   handleDotSaudio,
	".saudiosm": handleDotSaudio,
	"```saudio": handleDotSaudioConfig,
	"```toml":   handleDotSaudioConfig,
	".slimit":   handleDotSlimit,
}

// Subcommands for `.sim`
var simCommandHandlers = map[string]func() commands.CommandHandler{
	"arc":       func() commands.CommandHandler { return &image.ArcDistortCommand{} },
	"barrel":    func() commands.CommandHandler { return &image.BarrelDistortCommand{} },
	"ibarrel":   func() commands.CommandHandler { return &image.InverseBarrelDistortCommand{} },
	"polar":     func() commands.CommandHandler { return &image.PolarDistortCommand{} },
	"ipolar":    func() commands.CommandHandler { return &image.InversePolarDistortCommand{} },
	"genframes": func() commands.CommandHandler { return &image.GenFramesCommand{} },
}

const usage = `Usage: .saudio [flags] <prompt words>

  <prompt words>
        collection of all the non-flag strings that make up your prompt

Flags:
  --help, -h, --usage
        display this help message

  --negative
        if present, makes all of the prompt words that follow this flag negative

  --strength int
        how strongly the model follows your prompt
        default: 7    (turning it up can actually worsen quality)

  --seed int
        RNG seed for generation; default is a random positive integer

  --steps int
        number of diffusion iterations; default: 100
        note: values ≫100 rarely improve results and can hang the bot

  --length int
        length of the audio clip to generate, in seconds; default: 30
        best quality at ~30s; >85s may exhaust GPU VRAM
`

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
	for key := range simCommandHandlers {
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

	// if it doesn't have at least a top level command + argument, ignore it
	if len(parts) < 2 {
		return
	}

	// if it doesn't start with a registered command, ignore it
	topCommandHandler, ok := topCommandHandlers[parts[0]]
	if !ok {
		return
	}

	err := topCommandHandler(session, message)
	if err != nil {
		slog.Error("Command handler failed with error: %w", err)
		session.ChannelMessageSend(message.ChannelID, fmt.Sprintf("Received error while executing command: %v", err))
	}
}

func handleDotSim(session *discordgo.Session, message *discordgo.MessageCreate) error {
	if len(strings.TrimSpace(message.Content)) < 1 {
		return fmt.Errorf("tried to handle .sim command without any message content")
	}
	parts := strings.Fields(message.Content)
	commandString := parts[1]
	commandConstructor, ok := simCommandHandlers[commandString]
	if !ok {
		session.ChannelMessageSend(message.ChannelID, "Received unknown command '`"+commandString+"`'; must be one of '"+getCommandList()+"'")
		return nil
	}

	command := commandConstructor()
	command.SetContext(session, message)
	if err := command.Apply(); err != nil {
		return err
	}

	slog.Info("applying .sim command...")
	return nil
}

func handleDotSaudio(session *discordgo.Session, message *discordgo.MessageCreate) error {
	command := &audio.StableAudioCommand{}
	command.SetContext(session, message)

	// need to validate input before we can save the prompt
	if err := command.Validate(); err != nil {
		session.ChannelMessageSend(message.ChannelID, command.Usage())
		reported_err := fmt.Errorf("couldn't validate Stable Audio command: %v", err)
		slog.Error(reported_err)
		return reported_err
	}

	parts := strings.Fields(message.Content)

	// finally, set the prompt
	parts = append(parts, "--small")
	command.SetPrompt(strings.Join(parts[1:], " "))

	if slices.Contains(parts, "--help") || slices.Contains(parts, "-h") || slices.Contains(parts, "--usage") {
		session.ChannelMessageSend(message.ChannelID, "```\n"+usage+"\n```")
		return nil
	}

	if audioQueueView == nil {
		audioQueueView := *exec.NewTaskQueueView(&audioQueue, session, message.ChannelID)
		go UpdateQueueViewCallback(&audioQueueView)
	}

	slog.Info("applying saudio command...")
	audioQueue.Enqueue(command)
	return nil
}

func handleDotSaudioConfig(session *discordgo.Session, message *discordgo.MessageCreate) error {
	command := &audio.StableAudioWithConfigCommand{}
	command.SetContext(session, message)

	if audioQueueView == nil {
		audioQueueView := *exec.NewTaskQueueView(&audioQueue, session, message.ChannelID)
		go UpdateQueueViewCallback(&audioQueueView)
	}

	slog.Info("applying saudio w/ config command...")
	audioQueue.Enqueue(command)
	return nil
}

func handleDotSlimit(session *discordgo.Session, message *discordgo.MessageCreate) error {
	command := &audio.LimitCommand{}
	command.SetContext(session, message)

	slog.Info("applying .slimit command...")
	command.Apply()
	return nil
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
	slog.SetLevel(slog.LevelTrace)

	token, err := loadDiscordToken()
	if err != nil {
		slog.Error("error loading Discord token, ", err)
		return
	}

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
