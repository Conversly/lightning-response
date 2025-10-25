package response

import (
    "context"
    "errors"
    "net/url"
    "strings"

    "github.com/Conversly/db-ingestor/internal/config"
    "github.com/Conversly/db-ingestor/internal/llm"
    "github.com/Conversly/db-ingestor/internal/loaders"
    "github.com/Conversly/db-ingestor/internal/rag"
)

// Service orchestrates tenant resolution, flow initialization, and execution.
type Service struct {
    db  *loaders.PostgresClient
    cfg *config.Config
}

func NewService(db *loaders.PostgresClient, cfg *config.Config) *Service {
    return &Service{db: db, cfg: cfg}
}

// ValidateAndResolveTenant validates API key and origin domain.
// TODO: implement database-backed lookup mapping api key -> tenant and allowed domains.
func (s *Service) ValidateAndResolveTenant(ctx context.Context, apiKey string, originURL string) (tenantID string, err error) {
    if strings.TrimSpace(apiKey) == "" {
        return "", errors.New("missing API key")
    }
    if strings.TrimSpace(originURL) == "" {
        return "", errors.New("missing origin_url")
    }
    if _, err := url.ParseRequestURI(originURL); err != nil {
        return "", errors.New("invalid origin_url")
    }
    // Placeholder: accept any api key for now; wire real lookup later
    // Example: SELECT tenant_id, allowed_domains FROM businesses WHERE api_key = $1
    return "tenant_placeholder", nil
}

// BuildAndRunFlow builds a minimal flow with RAG retriever and a no-op LLM provider.
func (s *Service) BuildAndRunFlow(ctx context.Context, req *Request, tenantID string) (*Response, error) {
    // Defaults until DB-backed configs exist
    flowCfg := TenantFlowConfig{
        TenantID:     tenantID,
        SystemPrompt: "You are a helpful AI assistant.",
        Temperature:  0.4,
        Model:        "stub-model",
        TopK:         5,
    }

    provider := llm.NewNoopProvider()
    retriever := rag.NewNoopRetriever()
    factory := NewFlowFactory(provider, retriever)
    flow := factory.Build(flowCfg)

    out, err := flow.Run(ctx, FlowInput{
        Query:   req.Query,
        History: nil, // conversation not wired yet
    })
    if err != nil {
        return nil, err
    }

    return &Response{
        Mode:            req.Mode,
        Answer:          out.Answer,
        Sources:         out.Sources,
        Usage:           out.Usage,
        ConversationKey: req.User.UniqueClientID,
    }, nil
}
