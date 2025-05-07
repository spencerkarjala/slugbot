package helpers

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/bwmarrin/discordgo"
)

func GetMimeTypeFromURL(url string) (string, error) {
	resp, err := http.Head(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch MIME type: %w", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		return "", fmt.Errorf("no MIME type found for URL: %s", url)
	}
	return contentType, nil
}

func GetFileExtensionFromMimeType(mimeType string) (string, error) {
	switch mimeType {
	case "image/gif":
		return "gif", nil
	case "image/jpeg":
		return "jpg", nil
	case "image/png":
		return "png", nil
	case "image/webp":
		return "webp", nil
	case "image/bmp":
		return "bmp", nil
	case "video/mp4":
		return "mp4", nil
	case "video/webm":
		return "webm", nil
	case "video/ogg":
		return "ogv", nil
	case "video/avi":
		return "avi", nil
	case "video/mkv":
		return "mkv", nil
	case "video/quicktime":
		return "mov", nil
	case "video/x-flv":
		return "flv", nil
	case "audio/mpeg":
		return "mp3", nil
	case "audio/ogg":
		return "ogg", nil
	case "audio/wav":
		return "wav", nil
	case "audio/flac":
		return "flac", nil
	default:
		return "", fmt.Errorf("unsupported MIME type: %s", mimeType)
	}
}

func GetFileExtensionFromURL(imageURL string) (string, error) {
	mimeType, err := GetMimeTypeFromURL(imageURL)
	if err != nil {
		return "", fmt.Errorf("couldn't determine mime type: %w", err)
	}
	fileExtension, err := GetFileExtensionFromMimeType(mimeType)
	if err != nil {
		return "", fmt.Errorf("coudn't determine file extension: %w", err)
	}
	return fileExtension, nil
}

func DownloadImage(imageURL string) (string, error) {
	fileExtension, err := GetFileExtensionFromURL(imageURL)
	if err != nil {
		return "", fmt.Errorf("coudn't determine file extension: %w", err)
	}

	tmpFile, err := os.CreateTemp("", fmt.Sprintf("in-*.%s", fileExtension))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	resp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to copy image content: %w", err)
	}

	return tmpFile.Name(), nil
}

func PrepareImageFiles(session *discordgo.Session, msg *discordgo.MessageCreate) (inputPath string, outputPath string, cleanup func(), err error) {
	imageURL, err := GetImageReference(session, msg)
	if err != nil {
		return "", "", nil, fmt.Errorf("error getting image reference: %w", err)
	}
	fmt.Println("Got image URL at: ", imageURL)
	mimeType, _ := GetMimeTypeFromURL(imageURL)
	fmt.Println("Image URL had MIME type: ", mimeType)

	tmpIn, err := DownloadImage(imageURL)
	if err != nil {
		return "", "", nil, fmt.Errorf("error downloading image: %w", err)
	}
	fmt.Println("Created temp infile at: ", tmpIn)

	fileExtension, err := GetFileExtensionFromURL(imageURL)
	if err != nil {
		return "", "", nil, fmt.Errorf("coudn't determine file extension: %w", err)
	}

	tmpOut, err := os.CreateTemp("", fmt.Sprintf("out-*.%s", fileExtension))
	if err != nil {
		os.Remove(tmpIn)
		return "", "", nil, fmt.Errorf("error creating output file: %w", err)
	}
	tmpOut.Close()
	fmt.Println("Created temp outfile at: ", tmpOut.Name())

	cleanup = func() {
		os.Remove(tmpIn)
		os.Remove(tmpOut.Name())
	}

	return tmpIn, tmpOut.Name(), cleanup, nil
}
