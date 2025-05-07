package helpers

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func IsImageAttachment(attachment discordgo.MessageAttachment) bool {
	return strings.HasPrefix(attachment.ContentType, "image/")
}

func GetEmbedImageURL(embed *discordgo.MessageEmbed) string {
	if embed.Image != nil && embed.Image.URL != "" {
		return embed.Image.URL
	}
	if embed.Video != nil && embed.Video.URL != "" {
		return embed.Video.URL
	}

	// for thumbnails, it seems thumbnail.proxy_url > thumbnail.url > url for ease-of-use
	if embed.Type == "image" && embed.Thumbnail != nil && embed.Thumbnail.ProxyURL != "" {
		return embed.Thumbnail.ProxyURL
	}
	if embed.Type == "image" && embed.Thumbnail != nil && embed.Thumbnail.URL != "" {
		return embed.Thumbnail.URL
	}
	if embed.Type == "image" && embed.URL != "" {
		return embed.URL
	}
	return ""
}

func GetMessageImageURL(message *discordgo.Message) string {
	for _, attachment := range message.Attachments {
		if IsImageAttachment(*attachment) {
			return attachment.URL
		}
	}
	for _, embed := range message.Embeds {
		if imageURL := GetEmbedImageURL(embed); imageURL != "" {
			return imageURL
		}
	}
	return ""
}

func GetImageFromReferencedMessage(session *discordgo.Session, message *discordgo.MessageCreate) (string, error) {
	if message.MessageReference == nil {
		return "", fmt.Errorf("message is not a reply")
	}

	replyMessage, err := session.ChannelMessage(message.ChannelID, message.MessageReference.MessageID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch message that was replied to")
	}

	if imageURL := GetMessageImageURL(replyMessage); imageURL != "" {
		return url.QueryUnescape(imageURL)
	}

	return "", fmt.Errorf("no image attachment found in message that was replied to")
}

func GetImageFromRecentChatHistory(session *discordgo.Session, message *discordgo.MessageCreate) (string, error) {
	messages, err := session.ChannelMessages(message.ChannelID, 50, "", "", "")
	if err != nil {
		return "", fmt.Errorf("failed to search recent messages for images")
	}

	for _, msg := range messages {
		for _, attachment := range msg.Attachments {
			if IsImageAttachment(*attachment) {
				return attachment.URL, nil
			}
		}
		for _, embed := range msg.Embeds {
			if imageURL := GetEmbedImageURL(embed); imageURL != "" {
				return imageURL, nil
			}
		}
	}

	return "", fmt.Errorf("no image found in recent chat history")
}
func GetImageReference(session *discordgo.Session, message *discordgo.MessageCreate) (string, error) {
	if message.Author.Bot {
		return "", fmt.Errorf("no image found")
	}

	// If message is a reply, get the image from the message being replied to
	if message.MessageReference != nil {
		return GetImageFromReferencedMessage(session, message)
	}

	// Otherwise, get it from the recent chat history
	if imageURL, err := GetImageFromRecentChatHistory(session, message); err == nil {
		return url.QueryUnescape(imageURL)
	}

	return "", fmt.Errorf("couldn't any images in recent chat history")
}

func UploadImage(session *discordgo.Session, channelID, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file for uploading: %w", err)
	}
	defer file.Close()

	_, err = session.ChannelFileSend(channelID, "processed.jpg", file)
	if err != nil {
		return fmt.Errorf("failed to send file to discord: %w", err)
	}

	return nil
}
