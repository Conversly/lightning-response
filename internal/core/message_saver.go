package core

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
	"go.uber.org/zap"
)

type messageSaver struct {
	db            *loaders.PostgresClient
	ch            chan loaders.MessageRow
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
	stoppedCh     chan struct{}
}

var (
	msgSaver     *messageSaver
	msgSaverOnce sync.Once
)

const (
	defaultMsgBatchSize    = 1000
	defaultFlushInterval   = 500 * time.Millisecond
	defaultChannelCapacity = 10000
)

func initMessageSaver(db *loaders.PostgresClient) {
	msgSaverOnce.Do(func() {
		msgSaver = &messageSaver{
			db:            db,
			ch:            make(chan loaders.MessageRow, defaultChannelCapacity),
			batchSize:     defaultMsgBatchSize,
			flushInterval: defaultFlushInterval,
			stopCh:        make(chan struct{}),
			stoppedCh:     make(chan struct{}),
		}
		go msgSaver.run()
	})
}

func (w *messageSaver) run() {
	defer close(w.stoppedCh)
	batch := make([]loaders.MessageRow, 0, w.batchSize)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.db.BatchInsertMessages(ctx, batch); err != nil {
			utils.Zlog.Error("Failed to batch insert messages", zap.Error(err), zap.Int("count", len(batch)))
			// Best-effort: retry once
			if err2 := w.db.BatchInsertMessages(ctx, batch); err2 != nil {
				utils.Zlog.Error("Retry failed for batch insert messages", zap.Error(err2), zap.Int("count", len(batch)))
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case row := <-w.ch:
			batch = append(batch, row)
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.stopCh:
			// Drain channel
			for {
				select {
				case row := <-w.ch:
					batch = append(batch, row)
					if len(batch) >= w.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// StopMessageSaver gracefully stops the message saver
func StopMessageSaver() {
	if msgSaver == nil {
		return
	}
	close(msgSaver.stopCh)
	<-msgSaver.stoppedCh
}

// SaveConversationMessagesBackground saves messages asynchronously via batch insert
func SaveConversationMessagesBackground(ctx context.Context, db *loaders.PostgresClient, records ...MessageRecord) error {
	if db == nil {
		return nil
	}
	initMessageSaver(db)

	for _, r := range records {
		citations := r.Citations
		if citations == nil {
			citations = []string{}
		}

		// Determine topic ID for user messages
		topicID := ""
		if strings.ToLower(r.Role) == "user" {
			topicID = determineTopicID(ctx, db, r.ChatbotID, r.Message)
		}

		// Default to WIDGET if channel not specified
		channel := r.Channel
		if channel == "" {
			channel = ChannelWidget
		}

		row := loaders.MessageRow{
			ChatbotID:       r.ChatbotID,
			Citations:       citations,
			Type:            strings.ToLower(r.Role),
			Content:         r.Message,
			CreatedAt:       time.Now().UTC(),
			UniqueConvID:    r.UniqueClientID,
			UniqueMsgID:     r.MessageUID,
			TopicID:         topicID,
			Channel:         string(channel),
			ChannelMetadata: r.ChannelMetadata,
		}

		select {
		case msgSaver.ch <- row:
			// enqueued
		default:
			// queue full: fallback to direct insert asynchronously
			go func(r loaders.MessageRow) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = db.BatchInsertMessages(ctx, []loaders.MessageRow{r})
			}(row)
		}
	}
	return nil
}

// determineTopicID extracts keywords from message and matches to chatbot topics
func determineTopicID(ctx context.Context, db *loaders.PostgresClient, chatbotID string, message string) string {
	// Fetch chatbot topics
	info, err := db.GetChatbotInfoWithTopics(ctx, chatbotID)
	if err != nil {
		utils.Zlog.Debug("Failed to fetch chatbot topics for topic tagging",
			zap.String("chatbot_id", chatbotID),
			zap.Error(err))
		return ""
	}

	// If no topics configured, return empty string
	if len(info.Topics) == 0 {
		return ""
	}

	// Extract keywords from message
	keywords := utils.ExtractKeywords(message, 4)

	// Match keywords to topics (will use "other" topic as fallback)
	topicID := utils.MatchTopicFromKeywords(keywords, info.Topics)

	utils.Zlog.Debug("Message tagged with topic",
		zap.String("chatbot_id", chatbotID),
		zap.Strings("keywords", keywords),
		zap.String("topic_id", topicID))

	return topicID
}
