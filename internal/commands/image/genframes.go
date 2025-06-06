package image

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"slugbot/internal/commands"
	"slugbot/internal/helpers"
	"slugbot/internal/io/slog"
)

// GenFramesCommand creates an animation where each frame is the input image.
type GenFramesCommand struct {
	commands.Command
}

func (c *GenFramesCommand) Usage() string {
	return "Usage: `.sim genframes <num_frames>`"
}

func (c *GenFramesCommand) Validate() error {
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
	if args[1] != "genframes" {
		return errors.New(c.Usage())
	}
	n, err := strconv.Atoi(args[2])
	if err != nil || n < 1 {
		return errors.New(c.Usage())
	}
	return nil
}

func (cmd *GenFramesCommand) Apply() error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	args := strings.Fields(cmd.Message.Content)
	frameCount, _ := strconv.Atoi(args[2])

	imageURL, err := helpers.GetImageReference(cmd.Session, cmd.Message)
	if err != nil {
		return fmt.Errorf("error getting image reference: %w", err)
	}

	inFile, err := helpers.DownloadImage(imageURL)
	if err != nil {
		return fmt.Errorf("error downloading image: %w", err)
	}

	paletteTmp, err := os.CreateTemp("", "palette-*.png")
	if err != nil {
		os.Remove(inFile)
		return fmt.Errorf("error creating palette file: %w", err)
	}
	paletteTmp.Close()
	paletteFile := paletteTmp.Name()

	outTmp, err := os.CreateTemp("", "out-*.gif")
	if err != nil {
		os.Remove(inFile)
		os.Remove(paletteFile)
		return fmt.Errorf("error creating output file: %w", err)
	}
	outTmp.Close()
	outFile := outTmp.Name()

	cleanup := func() {
		os.Remove(inFile)
		os.Remove(paletteFile)
		os.Remove(outFile)
	}
	defer cleanup()

	slog.Info("Running palette generation for the input file...")

	paletteGenCommand := exec.Command(
		"ffmpeg",
		"-i", inFile,
		"-vf", "palettegen",
		"-y", paletteFile,
	)

	slog.Trace(fmt.Sprintf("Running command: %s", strings.Join(paletteGenCommand.Args, " ")))

	if out, err := paletteGenCommand.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to generate palette on image: %w\nOutput: %s", err, string(out))
	}

	slog.Info(fmt.Sprintf("Duplicating image %s for %d frames...", inFile, frameCount))

	command := exec.Command(
		"ffmpeg",
		"-stream_loop", "-1",
		"-i", inFile,
		"-i", paletteFile,
		"-frames:v", fmt.Sprintf("%d", frameCount),
		"-filter_complex", fmt.Sprintf("[0:v]fps=%d[x];[x][1:v]paletteuse=dither=floyd_steinberg", frameCount),
		"-loop", "0",
		"-y", outFile,
	)

	slog.Trace(fmt.Sprintf("Running command: %s", strings.Join(command.Args, " ")))

	if out, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run command on image: %w\nOutput: %s", err, string(out))
	}

	slog.Trace("Finished running command; uploading image.")

	if err = helpers.UploadImage(cmd.Session, cmd.Message.ChannelID, outFile); err != nil {
		return fmt.Errorf("error uploading image: %w", err)
	}

	slog.Trace("Finished uploading image.")

	return nil
}
