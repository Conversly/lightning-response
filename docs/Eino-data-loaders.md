Splitter - markdown
Introduction
The Markdown Splitter is an implementation of the Document Transformer interface, used to split a Markdown document based on the document’s header hierarchy. This component implements the Eino: Document Transformer guide.

Working Principle
The Markdown Header Splitter works through the following steps:

Identify Markdown headers in the document (#, ##, ###, etc.)
Construct a document structure tree based on the header hierarchy
Split the document into independent segments based on the headers
Usage
Component Initialization
The Markdown Header Splitter is initialized using the NewHeaderSplitter function. The main configuration parameters are as follows:

splitter, err := markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
    Headers: map[string]string{
        "#":   "h1",              // Level 1 header
        "##":  "h2",              // Level 2 header
        "###": "h3",              // Level 3 header
    },
    TrimHeaders: false,           // Whether to keep header lines in the output
})
Explanation of configuration parameters:

Headers: Required parameter, defines the mapping between header tags and corresponding metadata key names
TrimHeaders: Whether to remove header lines from the output content
Full Usage Example
package main

import (
    "context"
    
    "github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
    "github.com/cloudwego/eino/schema"
)

func main() {
    ctx := context.Background()
    
    // Initialize the splitter
    splitter, err := markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
        Headers: map[string]string{
            "#":   "h1",
            "##":  "h2",
            "###": "h3",
        },
        TrimHeaders: false,
    })
    if err != nil {
        panic(err)
    }
    
    // Prepare the document to be split
    docs := []*schema.Document{
        {
            ID: "doc1",
            Content: `# Document Title

This is the content of the introduction section.

## Chapter 1

This is the content of Chapter 1.

### Section 1.1

This is the content of Section 1.1.

## Chapter 2

This is the content of Chapter 2.

\`\`\`
# This is a comment inside a code block and will not be recognized as a header
\`\`\`
`,
        },
    }
    
    // Execute the split
    results, err := splitter.Transform(ctx, docs)
    if err != nil {
        panic(err)
    }
    
    // Process the split results
    for i, doc := range results {
        println("Segment", i+1, ":", doc.Content)
        println("Header Hierarchy:")
        for k, v := range doc.MetaData {
            if k == "h1" || k == "h2" || k == "h3" {
                println("  ", k, ":", v)
            }
        }
    }
}





2. 

Loader - amazon s3
Introduction
The S3 Document Loader is an implementation of the Document Loader interface, used to load document content from AWS S3 buckets. This component implements the Eino: Document Loader guide.

Introduction to AWS S3 Service
Amazon Simple Storage Service (Amazon S3) is an object storage service offering industry-leading scalability, data availability, security, and performance. This component interacts with the S3 service using the AWS SDK for Go v2 and supports authentication through access keys or default credentials.

Usage
Component Initialization
The S3 Document Loader is initialized via the NewS3Loader function with the following main configuration parameters:

import (
  "github.com/cloudwego/eino-ext/components/document/loader/s3"
)

func main() {
    loader, err := s3.NewS3Loader(ctx, &s3.LoaderConfig{
        Region:           aws.String("us-east-1"),        // AWS Region
        AWSAccessKey:     aws.String("your-access-key"),  // AWS Access Key ID
        AWSSecretKey:     aws.String("your-secret-key"),  // AWS Secret Access Key
        UseObjectKeyAsID: true,                           // Whether to use the object key as the document ID
        Parser:           &parser.TextParser{},           // Document parser, defaults to TextParser
    })
}
Configuration parameter descriptions:

Region: The AWS region where the S3 bucket is located
AWSAccessKey and AWSSecretKey: AWS access credentials; if not provided, the default credential chain will be used
UseObjectKeyAsID: Whether to use the S3 object’s key as the document ID
Parser: The parser used for parsing document content, defaults to TextParser to directly convert content to a string
Loading Documents
Documents are loaded through the Load method:

docs, err := loader.Load(ctx, document.Source{
    URI: "s3://bucket-name/path/to/document.txt",
})
URI format description:

Must start with s3://
Followed by the bucket name and object key
Example: s3://my-bucket/folder/document.pdf
Precautions:

Currently, batch loading of documents via prefix is not supported
The URI must point to a specific object and cannot end with /
Ensure sufficient permissions to access the specified bucket and object
Complete Usage Example
Standalone Usage
package main

import (
    "context"
    
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/cloudwego/eino-ext/components/document/loader/s3"
    "github.com/cloudwego/eino/components/document"
)

func main() {
    ctx := context.Background()

    loader, err := s3.NewS3Loader(ctx, &s3.LoaderConfig{
        Region:           aws.String("us-east-1"),
        AWSAccessKey:     aws.String("your-access-key"),
        AWSSecretKey:     aws.String("your-secret-key"),
        UseObjectKeyAsID: true,
    })
    if err != nil {
        panic(err)
    }
    
    // Loading documents
    docs, err := loader.Load(ctx, document.Source{
        URI: "s3://my-bucket/documents/sample.txt",
    })
    if err != nil {
        panic(err)
    }
    
    // Using document content
    for _, doc := range docs {
        println(doc.Content)
    }
}



3. 


Loader - web url
Basic Introduction
The URL Document Loader is an implementation of the Document Loader interface, used to load document content from web URLs. This component implements the Eino: Document Loader guide.

Feature Introduction
The URL Document Loader has the following features:

Default support for HTML web content parsing
Customizable HTTP client configurations (e.g., custom proxies, etc.)
Supports custom content parsers (e.g., body, or other specific containers)
Usage
Component Initialization
The URL Document Loader is initialized using the NewLoader function with the main configuration parameters as follows:

import (
  "github.com/cloudwego/eino-ext/components/document/loader/url"
)

func main() {
    loader, err := url.NewLoader(ctx, &url.LoaderConfig{
        Parser:         parser,
        Client:         httpClient,
        RequestBuilder: requestBuilder,
    })
}
Explanation of configuration parameters:

Parser: Document parser, defaults to the HTML parser, which extracts the main content of the web page
Client: HTTP client which can be customized with timeout, proxy, and other configurations
RequestBuilder: Request builder used to customize request methods, headers, etc.
Loading Documents
Documents are loaded through the Load method:

docs, err := loader.Load(ctx, document.Source{
    URI: "https://example.com/document",
})
Note:

The URI must be a valid HTTP/HTTPS URL
The default request method is GET
If other HTTP methods or custom headers are needed, configure the RequestBuilder, for example in authentication scenarios
Complete Usage Example
Basic Usage
package main

import (
    "context"
    
    "github.com/cloudwego/eino-ext/components/document/loader/url"
    "github.com/cloudwego/eino/components/document"
)

func main() {
    ctx := context.Background()
    
    // Initialize the loader with default configuration
    loader, err := url.NewLoader(ctx, nil)
    if (err != nil) {
        panic(err)
    }
    
    // Load documents
    docs, err := loader.Load(ctx, document.Source{
        URI: "https://example.com/article",
    })
    if (err != nil) {
        panic(err)
    }
    
    // Use document content
    for _, doc := range docs {
        println(doc.Content)
    }
}
Custom Configuration Example
package main

import (
    "context"
    "net/http"
    "time"
    
    "github.com/cloudwego/eino-ext/components/document/loader/url"
    "github.com/cloudwego/eino/components/document"
)

func main() {
    ctx := context.Background()
    
    // Custom HTTP client
    client := &http.Client{
        Timeout: 10 * time.Second,
    }
    
    // Custom request builder
    requestBuilder := func(ctx context.Context, src document.Source, opts ...document.LoaderOption) (*http.Request, error) {
        req, err := http.NewRequestWithContext(ctx, "GET", src.URI, nil)
        if err != nil {
            return nil, err
        }
        // Add custom headers
        req.Header.Add("User-Agent", "MyBot/1.0")
        return req, nil
    }
    
    // Initialize the loader
    loader, err := url.NewLoader(ctx, &url.LoaderConfig{
        Client:         client,
        RequestBuilder: requestBuilder,
    })
    if (err != nil) {
        panic(err)
    }
    
    // Load documents
    docs, err := loader.Load(ctx, document.Source{
        URI: "https://example.com/article",
    })
    if (err != nil) {
        panic(err)
    }
    
    // Use document content
    for _, doc := range docs {
        println(doc.Content)
    }
}





4. 


Parser - html
Basic Introduction
The HTML Document Parser is an implementation of the Document Parser interface, used to parse the content of HTML web pages into plain text. This component implements the Eino: Document Parser guide, mainly used in the following scenarios:

When plain text content needs to be extracted from web pages
When metadata of web pages (title, description, etc.) needs to be retrieved
Feature Introduction
The HTML parser has the following features:

Supports selective extraction of page content with flexible content selector configuration (html selector)
Automatically extracts web page metadata (metadata)
Secure HTML parsing
Usage
Component Initialization
The HTML parser is initialized using the NewParser function, with the main configuration parameters listed below:

import (
  "github.com/cloudwego/eino-ext/components/document/parser/html"
)

parser, err := html.NewParser(ctx, &html.Config{
    Selector: &selector, // Optional: content selector, defaults to body
})
Configuration parameter description:

Selector: Optional parameter, specifies the content area to extract, using goquery selector syntax
For example: body indicates extracting the content of the <body> tag
#content indicates extracting the content of the element with id “content”
Metadata Description
The parser will automatically extract the following metadata:

html.MetaKeyTitle ("_title"): Webpage title
html.MetaKeyDesc ("_description"): Webpage description
html.MetaKeyLang ("_language"): Webpage language
html.MetaKeyCharset ("_charset"): Character encoding
html.MetaKeySource ("_source"): Document source URI
Complete Usage Example
Basic Usage
package main

import (
    "context"
    "strings"
    
    "github.com/cloudwego/eino-ext/components/document/parser/html"
    "github.com/cloudwego/eino/components/document/parser"
)

func main() {
    ctx := context.Background()
    
    // Initialize parser
    p, err := html.NewParser(ctx, nil) // Use default configuration
    if (err != nil) {
        panic(err)
    }
    
    // HTML content
    html := `
    <html lang="zh">
        <head>
            <title>Sample Page</title>
            <meta name="description" content="This is a sample page">
            <meta charset="UTF-8">
        </head>
        <body>
            <div id="content">
                <h1>Welcome</h1>
                <p>This is the main content.</p>
            </div>
        </body>
    </html>
    `
    
    // Parse the document
    docs, err := p.Parse(ctx, strings.NewReader(html),
        parser.WithURI("https://example.com"),
        parser.WithExtraMeta(map[string]any{
            "custom": "value",
        }),
    )
    if (err != nil) {
        panic(err)
    }
    
    // Use the parsing results
    doc := docs[0]
    println("Content:", doc.Content)
    println("Title:", doc.MetaData[html.MetaKeyTitle])
    println("Description:", doc.MetaData[html.MetaKeyDesc])
    println("Language:", doc.MetaData[html.MetaKeyLang])
}
Using Selector
package main

import (
    "context"
    
    "github.com/cloudwego/eino-ext/components/document/parser/html"
)

func main() {
    ctx := context.Background()
    
    // Specify to only extract the content of the element with id "content"
    selector := "#content"
    p, err := html.NewParser(ctx, &html.Config{
        Selector: &selector,
    })
    if (err != nil) {
        panic(err)
    }
    
    // ... code to parse the document ...
}







5. 


Parser - pdf
Introduction
The PDF Document Parser is an implementation of the Document Parser interface used to parse the contents of PDF files into plain text. This component implements the Eino: Document Loader guide and is mainly used for the following scenarios:

When you need to convert PDF documents into a processable plain text format
When you need to split the contents of a PDF document by page
Features
The PDF parser has the following features:

Supports basic PDF text extraction
Optionally splits documents by page
Automatically handles PDF fonts and encoding
Supports multi-page PDF documents
Notes:

May not fully support all PDF formats currently
Will not retain formatting like spaces and line breaks
Complex PDF layouts may affect extraction results
Usage
Component Initialization
The PDF parser is initialized using the NewPDFParser function, with the main configuration parameters as follows:

import (
  "github.com/cloudwego/eino-ext/components/document/parser/pdf"
)

func main() {
    parser, err := pdf.NewPDFParser(ctx, &pdf.Config{
        ToPages: true,  // Whether to split the document by page
    })
}
Configuration parameters description:

ToPages: Whether to split the PDF into multiple documents by page, default is false
Parsing Documents
Document parsing is done using the Parse method:

docs, err := parser.Parse(ctx, reader, opts...)
Parsing options:

Supports setting the document URI using parser.WithURI
Supports adding extra metadata using parser.WithExtraMeta
Complete Usage Example
Basic Usage
package main

import (
    "context"
    "os"
    
    "github.com/cloudwego/eino-ext/components/document/parser/pdf"
    "github.com/cloudwego/eino/components/document/parser"
)

func main() {
    ctx := context.Background()
    
    // Initialize the parser
    p, err := pdf.NewPDFParser(ctx, &pdf.Config{
        ToPages: false, // Do not split by page
    })
    if err != nil {
        panic(err)
    }
    
    // Open the PDF file
    file, err := os.Open("document.pdf")
    if err != nil {
        panic(err)
    }
    defer file.Close()
    
    // Parse the document
    docs, err := p.Parse(ctx, file, 
        parser.WithURI("document.pdf"),
        parser.WithExtraMeta(map[string]any{
            "source": "./document.pdf",
        }),
    )
    if err != nil {
        panic(err)
    }
    
    // Use the parsed results
    for _, doc := range docs {
        println(doc.Content)
    }
}





6. 