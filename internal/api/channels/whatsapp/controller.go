package whatsapp

import (
	"context"
	"net/http"

	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Controller handles WhatsApp webhook requests
type Controller struct {
	adapter *Adapter
	service *Service
}

// NewController creates a new WhatsApp controller
func NewController(adapter *Adapter, service *Service) *Controller {
	return &Controller{
		adapter: adapter,
		service: service,
	}
}

// WhatsAppWebhookPayload represents the incoming webhook from Meta
type WhatsAppWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text,omitempty"`
				} `json:"messages"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

// Webhook handles incoming WhatsApp webhook messages
// POST /whatsapp/webhook/:chatbotId
func (c *Controller) Webhook(ctx *gin.Context) {
	chatbotID := ctx.Param("chatbotId")

	// 1. Parse webhook payload
	var payload WhatsAppWebhookPayload
	if err := ctx.ShouldBindJSON(&payload); err != nil {
		utils.Zlog.Error("Failed to parse WhatsApp webhook payload",
			zap.String("chatbot_id", chatbotID),
			zap.Error(err))
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_payload",
		})
		return
	}

	// 2. Validate payload structure
	if len(payload.Entry) == 0 || len(payload.Entry[0].Changes) == 0 {
		utils.Zlog.Warn("Empty WhatsApp webhook payload",
			zap.String("chatbot_id", chatbotID))
		ctx.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	change := payload.Entry[0].Changes[0]
	value := change.Value

	// Check if there are messages
	if len(value.Messages) == 0 {
		utils.Zlog.Debug("No messages in webhook payload",
			zap.String("chatbot_id", chatbotID))
		ctx.JSON(http.StatusOK, gin.H{"status": "no_messages"})
		return
	}

	// 3. Extract key information
	phoneNumberID := value.Metadata.PhoneNumberID
	displayPhoneNumber := value.Metadata.DisplayPhoneNumber
	message := value.Messages[0]
	userWaID := message.From
	messageText := message.Text.Body
	messageType := message.Type
	incomingMsgID := message.ID

	// Extract user profile name if available
	var userDisplayName string
	if len(value.Contacts) > 0 {
		userDisplayName = value.Contacts[0].Profile.Name
	}

	utils.Zlog.Info("Received WhatsApp message",
		zap.String("chatbot_id", chatbotID),
		zap.String("phone_number_id", phoneNumberID),
		zap.String("user_wa_id", userWaID),
		zap.String("user_name", userDisplayName),
		zap.String("message_type", messageType),
		zap.String("message_id", incomingMsgID))

	// 4. Only process text messages for now
	if messageType != "text" {
		utils.Zlog.Debug("Ignoring non-text message",
			zap.String("chatbot_id", chatbotID),
			zap.String("message_type", messageType))
		ctx.JSON(http.StatusOK, gin.H{"status": "unsupported_type"})
		return
	}

	// 5. TODO: Verify Meta signature (X-Hub-Signature-256)
	// signature := ctx.GetHeader("X-Hub-Signature-256")
	// if !verifySignature(signature, payload) { ... }

	// 6. Get WhatsApp account and validate it belongs to this chatbot
	waAccount, err := c.adapter.db.GetWhatsAppAccountByPhoneNumberID(context.Background(), phoneNumberID)
	if err != nil {
		utils.Zlog.Error("Failed to get WhatsApp account",
			zap.String("chatbot_id", chatbotID),
			zap.String("phone_number_id", phoneNumberID),
			zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "whatsapp_account_not_found",
		})
		return
	}

	// Validate that the WhatsApp account belongs to the requested chatbot
	if waAccount.ChatbotID != chatbotID {
		utils.Zlog.Warn("WhatsApp account chatbot mismatch",
			zap.String("requested_chatbot_id", chatbotID),
			zap.String("account_chatbot_id", waAccount.ChatbotID),
			zap.String("phone_number_id", phoneNumberID))
		ctx.JSON(http.StatusForbidden, gin.H{
			"error": "chatbot_mismatch",
		})
		return
	}

	// 7. Respond immediately to Meta (they require fast response)
	ctx.JSON(http.StatusOK, gin.H{"status": "received"})

	// 8. Process message and send response in background
	go func() {
		processCtx := context.Background()

		req := &ProcessMessageRequest{
			ChatbotID:          chatbotID,
			PhoneNumberID:      phoneNumberID,
			UserWaID:           userWaID,
			UserPhoneNumber:    userWaID, // wa_id is the phone number
			UserDisplayName:    userDisplayName,
			MessageText:        messageText,
			IncomingMsgID:      incomingMsgID,
			AccessToken:        waAccount.AccessToken,
			WabaID:             waAccount.WabaID,
			DisplayPhoneNumber: displayPhoneNumber,
		}

		if err := c.service.ProcessAndRespond(processCtx, req); err != nil {
			utils.Zlog.Error("Failed to process WhatsApp message",
				zap.String("chatbot_id", chatbotID),
				zap.String("user_wa_id", userWaID),
				zap.Error(err))
		}
	}()
}

// VerifyWebhook handles Meta's webhook verification
// GET /whatsapp/webhook/:chatbotId
func (c *Controller) VerifyWebhook(ctx *gin.Context) {
	chatbotId := ctx.Param("chatbotId")

	// Meta sends these query parameters for verification:
	// - hub.mode: should be "subscribe"
	// - hub.verify_token: should match your configured token
	// - hub.challenge: echo this back to confirm

	mode := ctx.Query("hub.mode")
	token := ctx.Query("hub.verify_token")
	challenge := ctx.Query("hub.challenge")

	// TODO: Implement actual token verification from config based on chatbotId
	_ = mode
	_ = token
	_ = chatbotId

	// For now, just echo the challenge (placeholder)
	if challenge != "" {
		ctx.String(http.StatusOK, challenge)
		return
	}

	ctx.JSON(http.StatusForbidden, gin.H{
		"error": "verification_failed",
	})
}
