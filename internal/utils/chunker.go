package utils

// import (
// 	"context"
// 	"fmt"

// 	"github.com/cloudwego/eino/pkg/document"
// )

// func ProcessDocuments(ctx context.Context, req ProcessRequest) ([]Chunk, error) {
// 	var allChunks []Chunk

// 	// 1️⃣ Website Loading
// 	if len(req.WebsiteURLs) > 0 {
// 		htmlLoader := document.NewWebLoader() // Eino built-in
// 		for _, url := range req.WebsiteURLs {
// 			doc, _ := htmlLoader.Load(ctx, url)
// 			parser := document.NewHTMLParser()
// 			parsed, _ := parser.Parse(ctx, doc)
// 			splitter := document.NewRecursiveSplitter(1000, 200)
// 			chunks, _ := splitter.Split(ctx, parsed)
// 			allChunks = append(allChunks, convertChunks(chunks, url)...)
// 		}
// 	}

// 	// 2️⃣ PDF / DOCX Documents
// 	for _, file := range req.Documents {
// 		fileLoader := document.NewFileLoader()
// 		doc, _ := fileLoader.Load(ctx, file)
// 		parser := document.NewAutoParser() // detects file type
// 		parsed, _ := parser.Parse(ctx, doc)
// 		splitter := document.NewRecursiveSplitter(1000, 200)
// 		chunks, _ := splitter.Split(ctx, parsed)
// 		allChunks = append(allChunks, convertChunks(chunks, file.Filename)...)
// 	}

// 	// 3️⃣ Q&A Data (directly as text)
// 	for _, qa := range req.QnAData {
// 		text := fmt.Sprintf("Question: %s\nAnswer: %s", qa.Question, qa.Answer)
// 		allChunks = append(allChunks, Chunk{
// 			Text:     text,
// 			Source:   "QnA",
// 			Metadata: qa.Metadata,
// 		})
// 	}

// 	return allChunks, nil
// }

// func convertChunks(cs []document.Chunk, source string) []Chunk {
// 	var result []Chunk
// 	for _, c := range cs {
// 		result = append(result, Chunk{
// 			Text:     c.Text,
// 			Source:   source,
// 			Metadata: map[string]interface{}{"source_type": "parsed"},
// 		})
// 	}
// 	return result
// }
