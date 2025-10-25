package response

import (
    "context"

    "github.com/Conversly/db-ingestor/internal/llm"
    "github.com/Conversly/db-ingestor/internal/rag"
)

// TenantFlowConfig represents the minimal config needed to build a flow.
type TenantFlowConfig struct {
    TenantID     string
    SystemPrompt string
    Temperature  float32
    Model        string
    TopK         int
}

// FlowInput is the input to the compiled flow
type FlowInput struct {
    Query   string
    History []llm.Message
}

// FlowOutput is the output from the flow execution
type FlowOutput struct {
    Answer  string
    Sources []Source
    Usage   *Usage
}

// Flow is a compiled runnable pipeline
type Flow interface {
    Run(ctx context.Context, in FlowInput) (FlowOutput, error)
}

// FlowFactory assembles a Flow from LLM + Tools (only RAG for now)
type FlowFactory struct {
    provider  llm.Provider
    retriever rag.Retriever
}

func NewFlowFactory(provider llm.Provider, retriever rag.Retriever) *FlowFactory {
    return &FlowFactory{provider: provider, retriever: retriever}
}

func (f *FlowFactory) Build(cfg TenantFlowConfig) Flow {
    return &agentFlow{
        cfg:       cfg,
        provider:  f.provider,
        retriever: f.retriever,
    }
}

// agentFlow is a minimal, non-Eino placeholder that wires LLM + RAG
type agentFlow struct {
    cfg       TenantFlowConfig
    provider  llm.Provider
    retriever rag.Retriever
}

func (a *agentFlow) Run(ctx context.Context, in FlowInput) (FlowOutput, error) {
    // 1) Retrieve context (skeleton, ignore errors)
    docs, _ := a.retriever.Retrieve(ctx, a.cfg.TenantID, in.Query, a.cfg.TopK)

    // 2) Build messages: system + user
    msgs := make([]llm.Message, 0, len(in.History)+2)
    if a.cfg.SystemPrompt != "" {
        msgs = append(msgs, llm.Message{Role: "system", Content: a.cfg.SystemPrompt})
    }
    msgs = append(msgs, in.History...)
    msgs = append(msgs, llm.Message{Role: "user", Content: in.Query})

    // 3) Call provider
    text, usage, err := a.provider.Generate(ctx, msgs, llm.ModelConfig{
        Model:        a.cfg.Model,
        Temperature:  a.cfg.Temperature,
        SystemPrompt: a.cfg.SystemPrompt,
    })
    if err != nil {
        return FlowOutput{}, err
    }

    // 4) Convert docs to sources
    sources := make([]Source, 0, len(docs))
    for _, d := range docs {
        sources = append(sources, Source{Title: d.Title, URL: d.URL, Snippet: d.Snippet})
    }

    return FlowOutput{
        Answer:  text,
        Sources: sources,
        Usage:   &Usage{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens},
    }, nil
}
