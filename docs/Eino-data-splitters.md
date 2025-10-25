Splitter - recursive
Basic Introduction
The Recursive Splitter is an implementation of the Document Transformer interface, used to recursively split long documents into smaller segments according to a specified size. This component implements the Eino: Document Transformer guide.

Working Principle
The Recursive Splitter works through the following steps:

Try to split the document in the order of the separator list
If the current separator cannot split the document into segments smaller than the target size, use the next separator
Merge the resulting segments to ensure the segment size is close to the target size
Maintain an overlapping region of the specified size during the merging process
Usage
Component Initialization
The recursive splitter is initialized via the NewSplitter function, with the main configuration parameters as follows:

splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
    ChunkSize:    1000,           // Required: Target chunk size
    OverlapSize:  200,            // Optional: Chunk overlap size
    Separators:   []string{"\n", ".", "?", "!"}, // Optional: List of separators
    LenFunc:      nil,            // Optional: Custom length calculation function
    KeepType:     recursive.KeepTypeNone, // Optional: Separator retention strategy
})
Configuration parameters explanation:

ChunkSize: Required parameter, specifies the target chunk size
OverlapSize: Overlap size between chunks, used to maintain context coherence
Separators: List of separators, used in order of priority
LenFunc: Custom text length calculation function, defaults to len()
KeepType: Separator retention strategy, optional values:
KeepTypeNone: Do not retain separators
KeepTypeStart: Retain separators at the start of the chunk
KeepTypeEnd: Retain separators at the end of the chunk
Complete Usage Example
package main

import (
    "context"
    
    "github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
    "github.com/cloudwego/eino/schema"
)

func main() {
    ctx := context.Background()
    
    // Initialize the splitter
    splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
        ChunkSize:   1000,
        OverlapSize: 200,
        Separators:  []string{"\n\n", "\n", "。", "！", "？"},
        KeepType:    recursive.KeepTypeEnd,
    })
    if err != nil {
        panic(err)
    }
    
    // Prepare the documents to be split
    docs := []*schema.Document{
        {
            ID: "doc1",
            Content: `这是第一个段落，包含了一些内容。
            
            这是第二个段落。这个段落有多个句子！这些句子通过标点符号分隔。
            
            这是第三个段落。这里有更多的内容。`,
        },
    }
    
    // Perform splitting
    results, err := splitter.Transform(ctx, docs)
    if err != nil {
        panic(err)
    }
    
    // Handle the split results
    for i, doc := range results {
        println("Chunk", i+1, ":", doc.Content)
    }
}
Advanced Usage
Custom length calculation:

splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
    ChunkSize: 1000,
    LenFunc: func(s string) int {
        // e.g.: Use the number of Unicode characters instead of bytes
        return len([]rune(s))
    },
})
Adjusting overlap strategy:

splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
    ChunkSize:   1000,
    // Increase overlap area to retain more context
    OverlapSize: 300,
    // Retain separator at the end of the chunk
    KeepType:    recursive.KeepTypeEnd,
})
Custom separators:

splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
    ChunkSize: 1000,
    // List of separators sorted by priority
    Separators: []string{
        "\n\n",     // Empty line (paragraph separator)
        "\n",       // Line break
        "。",       // Period
    },
})





2.



