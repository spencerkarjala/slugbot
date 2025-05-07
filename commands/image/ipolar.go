package image

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"test-discord-bot/commands"
	"test-discord-bot/helpers"
)

type InversePolarDistortCommand struct {
	commands.Command
}

func (c *InversePolarDistortCommand) Usage() string {
	return "Usage: `.im ipolar <A>`"
}

func (c *InversePolarDistortCommand) Validate() error {
	if c.Session == nil {
		return fmt.Errorf("invalid session reference")
	}
	if c.Message == nil {
		return fmt.Errorf("invalid message reference")
	}

	args := strings.Fields(c.Message.Content)

	if len(args) != 3 {
		return errors.New(c.Usage())
	}

	if args[1] != "ipolar" {
		return errors.New(c.Usage())
	}

	if _, err := strconv.ParseFloat(args[2], 64); err != nil {
		return errors.New(c.Usage())
	}

	return nil
}

func (cmd *InversePolarDistortCommand) Apply() error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	args := strings.Fields(cmd.Message.Content)
	theta, _ := strconv.ParseFloat(args[2], 64)

	inFile, outFile, cleanup, err := helpers.PrepareImageFiles(cmd.Session, cmd.Message)
	if err != nil {
		return err
	}
	defer cleanup()

	command := exec.Command(
		"magick",
		inFile,
		"-distort",
		"DePolar",
		fmt.Sprintf("%f", theta),
		outFile,
	)
	fmt.Println("Running command:", strings.Join(command.Args, " "))
	if out, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run command on image: %w\nOutput: %s", err, string(out))
	}

	if err = helpers.UploadImage(cmd.Session, cmd.Message.ChannelID, outFile); err != nil {
		return fmt.Errorf("error uploading image: %w", err)
	}

	return nil
}
