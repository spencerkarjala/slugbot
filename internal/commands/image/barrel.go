package image

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"slugbot/internal/commands"
	"slugbot/internal/helpers"
)

type BarrelDistortCommand struct {
	commands.Command
}

func (c *BarrelDistortCommand) Usage() string {
	return "Usage: `.im barrel <A> <B> <C> <D>`"
}

func (c *BarrelDistortCommand) Validate() error {
	if c.Session == nil {
		return fmt.Errorf("invalid session reference")
	}
	if c.Message == nil {
		return fmt.Errorf("invalid message reference")
	}

	args := strings.Fields(c.Message.Content)

	if len(args) != 6 {
		return errors.New(c.Usage())
	}

	if args[1] != "barrel" {
		return errors.New(c.Usage())
	}

	for i := 2; i < 6; i++ {
		if _, err := strconv.ParseFloat(args[i], 64); err != nil {
			return errors.New(c.Usage())
		}
	}

	return nil
}

func (cmd *BarrelDistortCommand) Apply() error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	args := strings.Fields(cmd.Message.Content)
	a, _ := strconv.ParseFloat(args[2], 64)
	b, _ := strconv.ParseFloat(args[3], 64)
	c, _ := strconv.ParseFloat(args[4], 64)
	d, _ := strconv.ParseFloat(args[5], 64)

	inFile, outFile, cleanup, err := helpers.PrepareImageFiles(cmd.Session, cmd.Message)
	if err != nil {
		return err
	}
	defer cleanup()

	command := exec.Command(
		"magick",
		inFile,
		"-distort",
		"Barrel",
		fmt.Sprintf("%f %f %f %f", a, b, c, d),
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
