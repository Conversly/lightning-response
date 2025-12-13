package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// VerifySignature verifies the Meta webhook signature
// The signature is in the format: sha256=<hex_signature>
func VerifySignature(signature string, payload []byte, appSecret string) error {
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format: missing sha256= prefix")
	}

	expectedSig := signature[7:] // Remove "sha256=" prefix

	// Compute HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(payload)
	computedSig := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison to prevent timing attacks
	if !hmac.Equal([]byte(expectedSig), []byte(computedSig)) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// WebhookPayload represents the structure of a Meta webhook payload
type WebhookPayload struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

// Entry represents a single entry in the webhook payload
type Entry struct {
	ID      string   `json:"id"`
	Changes []Change `json:"changes"`
}

// Change represents a change notification
type Change struct {
	Value Value  `json:"value"`
	Field string `json:"field"`
}

// Value contains the actual message data
type Value struct {
	MessagingProduct string    `json:"messaging_product"`
	Metadata         Metadata  `json:"metadata"`
	Contacts         []Contact `json:"contacts,omitempty"`
	Messages         []Message `json:"messages,omitempty"`
	Statuses         []Status  `json:"statuses,omitempty"`
}

// Metadata contains phone number information
type Metadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

// Contact represents a WhatsApp contact
type Contact struct {
	Profile Profile `json:"profile"`
	WaID    string  `json:"wa_id"`
}

// Profile contains contact profile information
type Profile struct {
	Name string `json:"name"`
}

// Message represents an incoming WhatsApp message
type Message struct {
	From      string       `json:"from"`
	ID        string       `json:"id"`
	Timestamp string       `json:"timestamp"`
	Type      string       `json:"type"`
	Text      *TextMessage `json:"text,omitempty"`
	Image     *MediaInfo   `json:"image,omitempty"`
	Document  *MediaInfo   `json:"document,omitempty"`
	Audio     *MediaInfo   `json:"audio,omitempty"`
	Video     *MediaInfo   `json:"video,omitempty"`
}

// TextMessage represents a text message
type TextMessage struct {
	Body string `json:"body"`
}

// MediaInfo represents media message details
type MediaInfo struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// Status represents a message status update
type Status struct {
	ID          string `json:"id"`
	Status      string `json:"status"` // sent, delivered, read, failed
	Timestamp   string `json:"timestamp"`
	RecipientID string `json:"recipient_id"`
}

