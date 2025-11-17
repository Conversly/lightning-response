package utils

import (
	"regexp"
	"sort"
	"strings"
)

var stopwords = map[string]bool{
	"i": true, "me": true, "my": true, "myself": true, "we": true, "our": true,
	"ours": true, "ourselves": true, "you": true, "your": true, "yours": true,
	"yourself": true, "yourselves": true, "he": true, "him": true, "his": true,
	"himself": true, "she": true, "her": true, "hers": true, "herself": true,
	"it": true, "its": true, "itself": true, "they": true, "them": true,
	"their": true, "theirs": true, "themselves": true, "what": true, "which": true,
	"who": true, "whom": true, "this": true, "that": true, "these": true,
	"those": true, "am": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "have": true, "has": true, "had": true,
	"having": true, "do": true, "does": true, "did": true, "doing": true,
	"a": true, "an": true, "the": true, "and": true, "but": true, "if": true,
	"or": true, "because": true, "as": true, "until": true, "while": true,
	"of": true, "at": true, "by": true, "for": true, "with": true, "about": true,
	"against": true, "between": true, "into": true, "through": true, "during": true,
	"before": true, "after": true, "above": true, "below": true, "to": true,
	"from": true, "up": true, "down": true, "in": true, "out": true, "on": true,
	"off": true, "over": true, "under": true, "again": true, "further": true,
	"then": true, "once": true, "here": true, "there": true, "when": true,
	"where": true, "why": true, "how": true, "all": true, "both": true,
	"each": true, "few": true, "more": true, "most": true, "other": true,
	"some": true, "such": true, "no": true, "nor": true, "not": true,
	"only": true, "own": true, "same": true, "so": true, "than": true,
	"too": true, "very": true, "can": true, "will": true, "just": true,
	"should": true, "now": true, "want": true, "need": true, "would": true,
	"could": true, "get": true, "got": true, "please": true, "help": true,
}

type keywordScore struct {
	word  string
	score float64
}

// ExtractKeywords extracts the top N keywords from a message using
// stopword filtering and frequency-based scoring
func ExtractKeywords(message string, topN int) []string {
	if message == "" {
		return []string{}
	}

	// Normalize and tokenize
	words := tokenize(message)
	if len(words) == 0 {
		return []string{}
	}

	// Count frequency and track first position
	wordFreq := make(map[string]int)
	wordFirstPos := make(map[string]int)

	for i, word := range words {
		word = strings.ToLower(word)

		// Skip if stopword or too short
		if stopwords[word] || len(word) < 3 {
			continue
		}

		wordFreq[word]++
		if _, exists := wordFirstPos[word]; !exists {
			wordFirstPos[word] = i
		}
	}

	if len(wordFreq) == 0 {
		return []string{}
	}

	// Calculate scores
	scores := make([]keywordScore, 0, len(wordFreq))
	totalWords := float64(len(words))

	for word, freq := range wordFreq {
		// Scoring factors:
		// 1. Frequency (more frequent = higher score)
		// 2. Length bonus (longer words often more meaningful)
		// 3. Position weight (earlier words slightly higher)

		freqScore := float64(freq) / totalWords
		lengthBonus := float64(len(word)) / 10.0
		positionWeight := 1.0 - (float64(wordFirstPos[word]) / totalWords * 0.3)

		score := (freqScore * 3.0) + lengthBonus + positionWeight

		scores = append(scores, keywordScore{
			word:  word,
			score: score,
		})
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Return top N
	result := make([]string, 0, topN)
	for i := 0; i < topN && i < len(scores); i++ {
		result = append(result, scores[i].word)
	}

	return result
}

// tokenize splits text into words
func tokenize(text string) []string {
	// Simple regex to split on non-alphanumeric characters
	re := regexp.MustCompile(`[^\w]+`)
	words := re.Split(text, -1)

	// Filter empty strings
	result := make([]string, 0, len(words))
	for _, w := range words {
		if w != "" {
			result = append(result, w)
		}
	}
	return result
}
