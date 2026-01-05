package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	ColorGreen = 3066993  // Success
	ColorBlue  = 5793266  // Info
	ColorRed   = 15158332 // Error
)

type DiscordWebhook struct {
	webhookURL string
}

func NewDiscordWebhook(webhookURL string) *DiscordWebhook {
	return &DiscordWebhook{
		webhookURL: webhookURL,
	}
}

// SendRipComplete sends a notification when disc ripping is complete
func (dw *DiscordWebhook) SendRipComplete(discName string, titlesRipped int, discType string) error {
	embed := map[string]interface{}{
		"title":       "✅ Rip Complete",
		"description": fmt.Sprintf("**%s** (%s)\n%d title(s) ripped and queued for encoding", discName, discType, titlesRipped),
		"color":       ColorGreen,
	}

	return dw.sendEmbed(embed)
}

// SendEncodeComplete sends a notification when encoding is complete
func (dw *DiscordWebhook) SendEncodeComplete(filename string, discType string) error {
	embed := map[string]interface{}{
		"title":       "🎬 Encode Complete",
		"description": fmt.Sprintf("**%s**\nProfile: %s → AV1", filename, discType),
		"color":       ColorBlue,
	}

	return dw.sendEmbed(embed)
}

// SendError sends an error notification
func (dw *DiscordWebhook) SendError(operation string, errorMsg string) error {
	embed := map[string]interface{}{
		"title":       "❌ Error",
		"description": fmt.Sprintf("**%s failed**\n```\n%s\n```", operation, errorMsg),
		"color":       ColorRed,
	}

	return dw.sendEmbed(embed)
}

// sendEmbed sends a Discord embed message
func (dw *DiscordWebhook) sendEmbed(embed map[string]interface{}) error {
	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{embed},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := http.Post(dw.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// SendMessage sends a simple text message (no embed)
func (dw *DiscordWebhook) SendMessage(message string) error {
	payload := map[string]string{
		"content": message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := http.Post(dw.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
