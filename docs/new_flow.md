1. route : 

/response

2. controller : 

Validates web_id + origin_url mapping. : use api key map from apikey-map.go file

query field have whole json array of prevous conversation in request payload.



3. service : 

checks graph cache

if not found : 

load chatbot config using chatbot id from postgres.  : postgres.go file have function for it. 

{
    	&info.ID,
		&info.Name,
		&info.Description,
		&info.SystemPrompt,
}
later we will add tools config in it too

4. create graphs.

for now only rag tool is available. implementation of actual retreiver is in rag/retreiver.go. 

start the Eino graph

get the final output, 

response will look like this : 

{
    response : "",
    citation : []string,
    success : true/false
}


then in background we will save the messages : 

{
    unique_client_id,
    chatbotid
    message : 
    role : user/assistant
    citations []string
}








### End-to-end flow (15 steps)

1. Route binding: `routes.SetupRoutes` → `response.RegisterRoutes` registers `POST /response` → `Controller.Respond`.


2. Controller entry: `Controller.Respond(*gin.Context)` reads JSON via `ctx.ShouldBindJSON(&Request)`.


3. Delegate to service: `Controller.Respond` calls `GraphService.BuildAndRunGraph(ctx.Request.Context(), &req)`.


4. Access validation: `BuildAndRunGraph` → `ValidateChatbotAccess(ctx, s.db, req.User.ConverslyWebID, req.Metadata.OriginURL)`.


5. API key/domain check: `ValidateChatbotAccess` ensures map is loaded `ApiKeyManager.LoadFromDatabase(ctx, db)`, extracts host via `extractHost`, then verifies with `ApiKeyManager.ValidateApiKeyAndDomain`.


6. Load chatbot config: `BuildAndRunGraph` loads from Postgres using `PostgresClient.GetChatbotInfo(ctx, req.ChatbotID)`.


7. Build runtime cfg: `BuildAndRunGraph` constructs `ChatbotConfig` (chatbot_id, system prompt, model, temperature, topK, tools).


8. Graph retrieval: `GetOrCreateTenantGraph(ctx, cfg)` checks `graphCache` and returns a compiled graph if present.


9. Graph build (cache miss): `buildTenantGraph(ctx, cfg)` creates model via `gemini.NewChatModel(ctx, &gemini.Config{...})`, sets up `compose.NewGraph`, `AddChatModelNode("model", ...)`, edges START→model→END, then `graph.Compile(ctx)`.


10. Parse conversation: `ParseConversationMessages(req.Query)` converts JSON array into `[]*schema.Message` using `schema.UserMessage`/`schema.AssistantMessage`.


11. Run graph: `invokeGraph(ctx, compiledGraph, messages, cfg)` calls `graph.Invoke(ctx, messages, model.WithTemperature(...), model.WithMaxTokens(...), gemini.WithTopK(...))`.


12. Extract citations: `extractCitations(result)` gathers citation URLs (currently best-effort/no-op).
13. Build API response: `BuildAndRunGraph` assembles `Response{response, citations, success=true}`.
14. Background persistence: goroutine calls `SaveConversationMessagesBackground(ctx, s.db, MessageRecord{user...}, MessageRecord{assistant...})`.
15. Send response: `Controller.Respond` attaches `request_id` from `middleware.RequestID` if present, then `ctx.JSON(http.StatusOK, result)`.

- The response payload fields are defined in `internal/api/response/schema.go` (`Response`).
- There’s no tenant layer; only `chatbot_id` drives config and graph selection.