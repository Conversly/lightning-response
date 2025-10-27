# Cache Removal Summary

## Overview
Removed all caching mechanisms from the graph engine to reduce code complexity and liability. Graphs and retrievers are now created fresh for each request.

## Files Modified

### 1. `internal/api/response/graph_engine.go`
**Removed:**
- `sync.Map` imports
- `graphCache` global variable (sync.Map)
- `retrieverCache` global variable (sync.Map)
- `globalTools` global variable (map)
- `globalToolsOnce` (sync.Once)
- `InitializeGraphEngine()` function
- `GetOrCreateChatbotGraph()` function (wrapper around cached graph)
- `buildChatbotGraph()` function (renamed to `BuildChatbotGraph`)
- `toolEnabled()` helper function - inlined single usage
- `lastUserContent()` helper function - inlined single usage
- `ClearGraphCache()` function
- `ClearAllGraphCaches()` function
- `GetCachedGraphCount()` function

**Changed:**
- Renamed `buildChatbotGraph()` to `BuildChatbotGraph()` (public)
- Added `GraphDependencies` struct for explicit dependency passing
- `BuildChatbotGraph()` now takes `deps *GraphDependencies` parameter
- Inlined RAG enablement check directly in state pre-handler
- Inlined last user message extraction in state pre-handler
- Retriever now created directly in pre-handler using `rag.NewPgVectorRetriever()`

**Lines of code:** Reduced from ~210 to ~140 (33% reduction)

---

### 2. `internal/api/response/graph_service.go`
**Removed:**
- Import of `github.com/cloudwego/eino-ext/components/model/gemini`
- Import of `github.com/cloudwego/eino/components/model`
- Call to `InitializeGraphEngine()` in `Initialize()` method

**Changed:**
- `Initialize()` now just logs and returns (no-op)
- `BuildAndRunGraph()` now calls `BuildChatbotGraph()` instead of `GetOrCreateChatbotGraph()`
- `BuildAndRunGraph()` now creates `GraphDependencies` struct
- `invokeGraph()` no longer passes model options (already set during model creation)

**Key Flow Change:**
```go
// BEFORE: Get cached or build new graph
compiledGraph, err := GetOrCreateChatbotGraph(ctx, cfg)

// AFTER: Always build new graph with explicit dependencies
deps := &GraphDependencies{
    DB:       s.db,
    Embedder: s.embedder,
}
compiledGraph, err := BuildChatbotGraph(ctx, cfg, deps)
```

---

### 3. `internal/api/response/graph_tools.go`
**Removed:**
- `globalDB` global variable
- `globalEmbedder` global variable
- Import of `github.com/Conversly/lightning-response/internal/embedder`
- Import of `github.com/Conversly/lightning-response/internal/loaders`
- Import of `github.com/Conversly/lightning-response/internal/rag`
- `SetGlobalDependencies()` function
- `getOrCreateRAGRetriever()` function (entire retriever caching logic)

**Kept:**
- `HTTPToolRequest` struct (for future use)
- `GetEnabledTools()` function (currently returns empty slice)

**Lines of code:** Reduced from ~70 to ~24 (66% reduction)

---

## Architectural Improvements

### Before (With Caching)
```
Request → GraphService
    ↓
    Check graphCache (sync.Map)
    ↓
    If miss → buildChatbotGraph()
        ↓
        Store in graphCache
    ↓
    Graph Pre-Handler
        ↓
        Check retrieverCache (sync.Map)
        ↓
        If miss → getOrCreateRAGRetriever()
            ↓
            Check globalDB/globalEmbedder
            ↓
            Create retriever
            ↓
            Store in retrieverCache
    ↓
    Invoke graph
```

### After (No Caching)
```
Request → GraphService
    ↓
    Create GraphDependencies{DB, Embedder}
    ↓
    BuildChatbotGraph(cfg, deps)
        ↓
        Create chat model
        ↓
        Graph Pre-Handler (inline)
            ↓
            Create retriever with deps
            ↓
            Retrieve documents
    ↓
    Invoke graph
```

---

## Benefits

### 1. **Reduced Complexity**
- **3 fewer global variables** (graphCache, retrieverCache, globalTools)
- **2 fewer sync primitives** (sync.Map × 2)
- **8 fewer functions** (cache management, wrappers, helpers)
- **~100 fewer lines of code** across 3 files

### 2. **Explicit Dependencies**
- No more global state (`globalDB`, `globalEmbedder`)
- Dependencies passed explicitly via `GraphDependencies` struct
- Easier to test and reason about

### 3. **Simpler Mental Model**
- One path: always build fresh
- No cache invalidation concerns
- No stale cache bugs
- No initialization ordering dependencies

### 4. **Better Observability**
- Every request creates new graph → clearer logs
- No confusion about cached vs fresh graphs
- Easier to trace request flow

---

## Performance Considerations

### Potential Impact
- **Graph compilation** happens every request (~1-5ms)
- **Retriever creation** happens every request (~negligible)
- **No memory savings** from reusing compiled graphs

### Mitigation
- Graph compilation is fast (Eino is optimized for this)
- Retriever is just a struct wrapper (no expensive initialization)
- Database connections are still pooled
- If performance becomes an issue, we can add caching back with proper metrics

---

## Testing Recommendations

1. **Load test** to measure actual performance impact
2. **Monitor** graph compilation time in production logs
3. **Compare** request latency before/after this change
4. **Verify** no memory leaks from graph/retriever creation

---

## Future Simplifications (Not Done Yet)

Based on the audit, these could be next:

1. **Remove `errorResponse()` wrapper** - inline 5 usages
2. **Simplify citation flow** - return citations directly instead of JSON suffix hack
3. **Remove `Retriever` interface** - only one implementation exists
4. **Flatten state handlers** - extract to named functions for testability
5. **Replace `MultiKeyChatModel`** - use single model with rotating client
6. **Move config to database** - Temperature, MaxTokens, Model currently hardcoded

---

## Rollback Plan

If performance degrades:
1. Revert commits affecting these 3 files
2. Re-enable `InitializeGraphEngine()` call in `graph_service.go`
3. Restore cache metrics logging

Git commits for this change:
- `Remove all graph and retriever caching`
