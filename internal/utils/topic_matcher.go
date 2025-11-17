package utils

import (
	"strings"

	"github.com/Conversly/lightning-response/internal/types"
)

const (
	MinSimilarityThreshold = 0.3
)

func MatchTopicFromKeywords(keywords []string, topics []types.ChatbotTopic) string {
	if len(topics) == 0 {
		return ""
	}

	if len(keywords) == 0 {
		return findOtherTopicID(topics)
	}

	bestScore := 0.0
	bestTopicID := ""

	// Compare keywords against each topic
	for _, topic := range topics {
		score := calculateTopicScore(keywords, topic.Name)
		if score > bestScore {
			bestScore = score
			bestTopicID = topic.ID
		}
	}

	// Return "other" topic ID if best score is below threshold
	if bestScore < MinSimilarityThreshold {
		return findOtherTopicID(topics)
	}

	return bestTopicID
}

// findOtherTopicID finds and returns the ID of the "other" topic
func findOtherTopicID(topics []types.ChatbotTopic) string {
	for _, topic := range topics {
		if strings.ToLower(topic.Name) == "other" {
			return topic.ID
		}
	}
	// Fallback to empty string if "other" topic not found
	return ""
}

// calculateTopicScore calculates similarity score between keywords and topic name
func calculateTopicScore(keywords []string, topicName string) float64 {
	topicNameLower := strings.ToLower(topicName)
	topicWords := strings.Fields(topicNameLower)

	if len(topicWords) == 0 {
		return 0.0
	}

	totalScore := 0.0
	matchCount := 0

	// For each keyword, find best match with topic words
	for _, keyword := range keywords {
		keywordLower := strings.ToLower(keyword)
		bestWordScore := 0.0

		for _, topicWord := range topicWords {
			similarity := calculateSimilarity(keywordLower, topicWord)
			if similarity > bestWordScore {
				bestWordScore = similarity
			}
		}

		if bestWordScore > 0 {
			totalScore += bestWordScore
			matchCount++
		}
	}

	// Average score weighted by match ratio
	if matchCount == 0 {
		return 0.0
	}

	avgScore := totalScore / float64(len(keywords))
	matchRatio := float64(matchCount) / float64(len(keywords))

	// Combine average score with match ratio
	return avgScore * (0.7 + 0.3*matchRatio)
}

// calculateSimilarity calculates similarity between two strings
// Returns a score between 0.0 and 1.0
func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// Check if one is substring of the other (high score)
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		shorter := len(s2)
		longer := len(s1)
		if len(s1) < len(s2) {
			shorter = len(s1)
			longer = len(s2)
		}
		return 0.8 * (float64(shorter) / float64(longer))
	}

	// Use Jaccard similarity for character n-grams
	return jaccardSimilarity(s1, s2, 2)
}

// jaccardSimilarity calculates Jaccard similarity using character n-grams
func jaccardSimilarity(s1, s2 string, n int) float64 {
	if len(s1) < n || len(s2) < n {
		// For very short strings, use simple character overlap
		return simpleCharOverlap(s1, s2)
	}

	ngrams1 := extractNGrams(s1, n)
	ngrams2 := extractNGrams(s2, n)

	if len(ngrams1) == 0 || len(ngrams2) == 0 {
		return 0.0
	}

	// Calculate intersection
	intersection := 0
	for ng := range ngrams1 {
		if ngrams2[ng] {
			intersection++
		}
	}

	// Calculate union
	union := len(ngrams1) + len(ngrams2) - intersection

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// extractNGrams extracts character n-grams from a string
func extractNGrams(s string, n int) map[string]bool {
	ngrams := make(map[string]bool)
	if len(s) < n {
		return ngrams
	}

	for i := 0; i <= len(s)-n; i++ {
		ngrams[s[i:i+n]] = true
	}
	return ngrams
}

// simpleCharOverlap calculates simple character overlap ratio
func simpleCharOverlap(s1, s2 string) float64 {
	chars1 := make(map[rune]bool)
	chars2 := make(map[rune]bool)

	for _, c := range s1 {
		chars1[c] = true
	}
	for _, c := range s2 {
		chars2[c] = true
	}

	overlap := 0
	for c := range chars1 {
		if chars2[c] {
			overlap++
		}
	}

	maxLen := len(chars1)
	if len(chars2) > maxLen {
		maxLen = len(chars2)
	}

	if maxLen == 0 {
		return 0.0
	}

	return float64(overlap) / float64(maxLen)
}
