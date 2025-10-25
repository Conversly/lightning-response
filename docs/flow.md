request : 


```

{
  "query": "How to speed up queries?",
  "user": {
    "unique_client_id": "721574583-1759770887",   // to group messages into a conversation
    "conversly_web_id": "e6a4437b-85c6-42f9-a447-f5bc8b6ca3f4",   // this is the api key
  },
  "metadata": {
    "origin_url": "https://clickhouse.com/docs".  // domain of website
  }
}

```


middleware : 
1. schema validation
2. validates if conversly_web_id and origin_url matches in our db. 
3. fetches chatbot id along with config using web_id


{
    systemPrompt,
    chatbotId,
    temperature,
    // more in future
}

4. initializes the Eino agent llm using system prompt, and chatbotId to initialize RAG tool.
5. then start the pipeline to get the response of the query. 
5. inserts the query and the resopnse of llm in message table in db

{
    unique client id,
    message 
    type : query/response
    chatbotId,
    origin_url
}



detailed flow :

# ⚙️ Conversly Response Pipeline — with Eino Integration

---

## 🧩 Request Example

```json
{
  "query": "How to speed up queries?",
  "user": {
    "unique_client_id": "721574583-1759770887",
    "conversly_web_id": "e6a4437b-85c6-42f9-a447-f5bc8b6ca3f4"
  },
  "metadata": {
    "origin_url": "https://clickhouse.com/docs"
  }
}
```

---

## 🧱 Pipeline Overview

| Step | Layer      | Description                                        |
| ---- | ---------- | -------------------------------------------------- |
| 1    | Middleware | Validate schema and input format                   |
| 2    | Middleware | Validate `conversly_web_id` + `origin_url` mapping |
| 3    | Middleware | Fetch chatbot config and metadata                  |
| 4    | Core       | Initialize Eino Flow + LLM + Tools                 |
| 5    | Core       | Run Flow (`flow.Run()`) to get response            |
| 6    | Storage    | Save query and response in DB                      |

---

## 🧠 Detailed Flow with Eino Integration

---

### **Step 1. Schema Validation**

**Purpose:** Ensure incoming JSON adheres to required structure.
You can use a Go validation lib like `go-playground/validator`:

```go
type QueryRequest struct {
    Query   string `json:"query" validate:"required"`
    User    struct {
        UniqueClientID string `json:"unique_client_id" validate:"required"`
        WebID          string `json:"conversly_web_id" validate:"required,uuid4"`
    } `json:"user"`
    Metadata struct {
        OriginURL string `json:"origin_url" validate:"required,url"`
    } `json:"metadata"`
}
```

---

### **Step 2. Domain & Web ID Validation**

**Purpose:** Verify the request originates from an authorized business.

```go
tenant, err := db.GetTenantByWebID(req.User.WebID)
if err != nil { return ErrUnauthorized }

if !tenant.AllowedDomains.Contains(req.Metadata.OriginURL) {
    return ErrForbidden
}
```

---

### **Step 3. Load Chatbot Config**

Retrieve all necessary data for initializing the flow.

```go
cfg, err := db.GetChatbotConfig(req.User.WebID)
// cfg → { chatbotId, systemPrompt, temperature, ragIndex, toolsConfig }
```

---

### **Step 4. Initialize Eino Flow**

This is the key step — where **Eino components** are assembled dynamically.

#### 🧩 Step 4.1 Initialize LLM

```go
import "github.com/cloudwego/eino/llms/gemini"

llm := openai.NewChatModel("gpt-4o-mini").
    WithTemperature(cfg.Temperature).
    WithSystemPrompt(cfg.SystemPrompt)
```

---

#### 🧩 Step 4.2 Initialize RAG Tool

You’ll create a RAG retriever using your business’s RAG source:

```go
import "github.com/cloudwego/eino/modules/rag"

ragTool := rag.NewRetriever(rag.RetrieverConfig{
    IndexName: cfg.RAGIndex,
    TopK:      5,
})
```

Eino’s `Retriever` tool conforms to the `Tool` interface automatically.

---

#### 🧩 Step 4.3 Initialize Other Tools Dynamically

If the tenant has custom tool definitions:

```go
tools := []tools.Tool{}
for _, t := range cfg.ToolsConfig {
    instance := NewSkeletonTool(t.Params)
    tools = append(tools, instance)
}
```

---

#### 🧩 Step 4.4 Create the React Agent

Use Eino’s `reactagent.NewReactAgent()` — this is the reasoning orchestrator.

```go
import "github.com/cloudwego/eino/modules/reactagent"

agent := reactagent.NewReactAgent(llm, append([]tools.Tool{ragTool}, tools...))
agent.SetPromptTemplate(cfg.SystemPrompt)
```

This agent:

* Handles the LLM reasoning loop
* Decides when to call which tool
* Feeds tool outputs back into the conversation

---

#### 🧩 Step 4.5 Build the Flow Graph

Every Eino execution must be part of a `flow.Flow` object.

```go
import "github.com/cloudwego/eino/flow"

f := flow.New()
f.AddNode("agent", agent)
f.SetEntrypoint("agent")
```

---

### **Step 5. Execute Flow**

Now the flow is ready to process user input.

```go
ctx := context.Background()
result, err := f.Run(ctx, map[string]interface{}{
    "query": req.Query,
})
if err != nil {
    log.Error(err)
    return ErrorResponse
}

responseText := result["output"].(string)
```

Internally, this will:

1. Call the LLM through `agent.Run()`
2. Use `reactagent`’s reasoning loop to decide if tools need to be called
3. Aggregate the final LLM output as `output`

---

### **Step 6. Store Conversation Messages**

Insert both query and response into your `messages` table:

```go
db.InsertMessage(Message{
    UniqueClientID: req.User.UniqueClientID,
    Type:           "query",
    Message:        req.Query,
    ChatbotID:      cfg.ChatbotID,
    OriginURL:      req.Metadata.OriginURL,
})

db.InsertMessage(Message{
    UniqueClientID: req.User.UniqueClientID,
    Type:           "response",
    Message:        responseText,
    ChatbotID:      cfg.ChatbotID,
    OriginURL:      req.Metadata.OriginURL,
})
```

---

## 🧩 Eino Components Used

| Eino Component                   | Package                                        | Role                                      |
| -------------------------------- | ---------------------------------------------- | ----------------------------------------- |
| **`flow.New()`**                 | `github.com/cloudwego/eino/flow`               | Builds execution graph                    |
| **`flow.Run()`**                 | `flow.Flow`                                    | Runs the entire pipeline                  |
| **`reactagent.NewReactAgent()`** | `github.com/cloudwego/eino/modules/reactagent` | Core reasoning engine                     |
| **`rag.NewRetriever()`**         | `github.com/cloudwego/eino/modules/rag`        | RAG data retriever                        |
| **`llm.OpenAIChatModel`**        | `github.com/cloudwego/eino/llms/openai`        | LLM interface                             |
| **`tools.Tool` interface**       | `github.com/cloudwego/eino/core/tools`         | Interface for custom tool implementations |

---

## ✅ Summary: Full Eino Request Lifecycle

1. **Validate request**
2. **Identify tenant**
3. **Fetch config**
4. **Initialize Eino components**

   * LLM → `openai.NewChatModel`
   * Tools → `rag.NewRetriever` + custom tools
   * Agent → `reactagent.NewReactAgent`
   * Flow → `flow.New() + AddNode()`
5. **Run Flow → `flow.Run()`**
6. **Persist query + response**
7. **Return JSON response**

---

Would you like me to write this into a **design document format** (with sections like “LLM Initialization”, “Tool Abstraction Layer”, “Flow Execution Lifecycle”, etc.) for internal documentation?
