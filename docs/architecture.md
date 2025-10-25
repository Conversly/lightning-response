
# 🧠 Conversly LLM Response Service — Architecture Overview

## 1️⃣ What We Are Building

### 🎯 Goal

A **multi-tenant LLM response microservice** that powers chatbots for many different businesses.
Each business can:

* Define its own **tools** (custom APIs or functions)
* Define its own **RAG data sources**
* Define its own **prompt templates**
* Serve queries from its own website via our **hosted chatbot widget**

The service receives queries from the end-users of these businesses and returns an intelligent, context-aware response — using the correct RAG data and tools configured for that specific business.

---

### 🧩 Core Capabilities

| Capability                      | Description                                                                                                         |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| **Tenant Isolation**            | Each business (tenant) has its own configuration, data, and tools. No leakage across tenants.                       |
| **Dynamic Flow Initialization** | On each request, a Flow is dynamically built using the business’s specific tools, RAG index, and prompts.           |
| **Customizable Tools**          | Tools are defined using a **Skeleton Tool Template**, which can be parameterized differently per business.          |
| **RAG Integration**             | Each business can register its own RAG data source (S3, Postgres, Vector DB, etc.) and embed retrieval logic.       |
| **LLM Reasoning (Eino)**        | Core reasoning engine powered by CloudWeGo Eino’s `reactagent`, managing tool calling and LLM responses.            |
| **Multi-Tenant Security**       | API authentication + domain validation ensures requests only come from authorized business widgets.                 |
| **Scalability**                 | Stateless Go microservice can horizontally scale, with tools and configs dynamically loaded from a shared database. |

---

### 🧠 Example Use Case

Suppose **Microsoft** and **Netflix** both use Conversly:

| Business  | Tools                                                              | RAG                      | Prompt                         |
| --------- | ------------------------------------------------------------------ | ------------------------ | ------------------------------ |
| Microsoft | 4 tools (e.g., Docs API, Product DB API, News Feed, FAQ Retriever) | Vector index in Pinecone | Brand-specific tone prompt     |
| Netflix   | 1 tool (e.g., Movie Search API)                                    | No RAG                   | Friendly conversational prompt |

When a user from `microsoft.com` chats with the widget:

1. Widget sends request → Response service
2. Service identifies tenant = Microsoft
3. Builds Flow with 4 tool instances + RAG retriever
4. Executes reasoning through Eino ReactAgent
5. Returns structured response JSON

---

## 2️⃣ How We Are Building It

### 🧱 Core Tech Stack

| Component        | Technology                                              |
| ---------------- | ------------------------------------------------------- |
| **Language**     | Go                                                      |
| **Framework**    | CloudWeGo Eino (for LLM, flow, and agent orchestration) |
| **LLM Provider** | Gemini (via unified LLM interface) |
| **Database**     | PostgreSQL (tenant configs, tool params, RAG metadata)  |
| **Vector Store** | pg vector (RAG retrieval)                       | 
| **Auth**         | API Key + Domain validation                             |

---

request body : 

```

{
  "query": "How to speed up queries?",
  "user": {
    "unique_client_id": "721574583-1759770887",   // to group messages into a conversation
    "kapa_web_id": "e6a4437b-85c6-42f9-a447-f5bc8b6ca3f4",   // this is the api key
  },
  "metadata": {
    "origin_url": "https://clickhouse.com/docs".  // domain of website
  }
}


```

### 🧩 High-Level Architecture

```
 ┌───────────────────────────────┐
 │         Client Website         │
 │  (Chatbot Widget on Domain)    │
 └──────────────┬────────────────┘
                │ HTTPS (Business API Key)
                ▼
 ┌──────────────────────────────────┐
 │     Conversly Response Service   │
 │  (Go + Eino Microservice)        │
 │----------------------------------│
 │  1. Auth & Tenant Resolution     │
 │  2. Flow Initialization          │
 │     - Load tenant config         │
 │     - Load tools (N instances)   │
 │     - Load RAG retriever         │
 │     - Set prompt template        │
 │  3. React Agent (LLM reasoning)  │
 │  4. Tool execution (Eino flow)   │
 │  5. Response serialization       │
 └────────────────┬────────────────┘
                  │
                  ▼
      ┌──────────────────────────┐
      │  Databases / Vector DB   │
      │  (Tenant configs, RAG)   │
      └──────────────────────────┘
```

---

### ⚙️ Flow Initialization Logic

For each request:

1. **Identify tenant**

   * Extract `x-api-key`
   * Validate + fetch tenant info from DB
   * Verify request origin domain
   * x api key will be mapped to domains allowed.
   * if mapping found, we continue, otherwise return 'unauthenticated error'

2. **Load configs**
    * using api key, we can find chatbot ID, we will use it to load whole config of chatbot along with tools params.
    * we will use the config to iitialize the tools, prompt template, rag functions
   * Tools (`business_tools` table)
   * RAG settings (`business_rag_sources`)
   * Prompt templates (`business_prompts`)

3. **Build Flow**

   ```go
   f := flow.New()
   ragTool := rag.NewRetriever(cfg.RAGIndex)
   customTools := makeToolsFromConfig(cfg.Tools)
   agent := reactagent.NewReactAgent(llm, append([]tools.Tool{ragTool}, customTools...))
   agent.SetPromptTemplate(cfg.Prompt)
   f.AddNode("agent", agent)
   f.SetEntrypoint("agent")
   ```

4. **Run Flow**

   ```go
   result, _ := f.Run(ctx, map[string]interface{}{
       "query": userInput,
   })
   ```

---

### 🔐 Security Model

| Mechanism              | Purpose                                                           |
| ---------------------- | ----------------------------------------------------------------- |
| **API Key per tenant** | Each chatbot widget uses a unique key identifying the business    |
| **Domain allowlist**   | Request only accepted from business’s verified website(s)         |
| **Rate limiting**      | Prevent abuse of endpoints                                        |
| **Signed responses**   | (Optional) If you want to ensure authenticity of response packets |
| **LLM call proxy**     | Only backend holds LLM API credentials, never exposed to clients  |

---

### 📦 Multi-Tenant Data Model (Simplified)

| Table                  | Purpose                                                       |
| ---------------------- | ------------------------------------------------------------- |
| `businesses`           | Stores basic info (id, name, allowed domains, api_key)        |
| `business_tools`       | Stores tool definitions (business_id, name, params, endpoint) |
| `business_prompts`     | Stores custom prompts per business                            |
| `business_rag_sources` | Stores RAG retrievers config per business                     |

---

### 🚀 Deployment Strategy

* Stateless Go microservice → deploy multiple replicas
* Load balance via NGINX or AWS ALB
* Cached LLM + vector clients (connection pooling)
* Use Redis for ephemeral flow caching if needed
* Observability via Prometheus + Grafana

---

### ✅ Example Lifecycle

1. **Tenant Onboarding**

   * Tenant registers → gets API key
   * Defines tools + RAG + prompts via admin dashboard

2. **User Query**

   * Widget sends query to `/v1/query` with API key + domain
   * Service initializes tenant flow
   * Runs reasoning and tools
   * Returns formatted response