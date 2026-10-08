package run

import (
	"context"
	"sync"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/toolindex"
)

// toolIndexCache holds the last dense/lexical catalog index for DP-2a so we
// do not re-embed hundreds of tool docs every ReAct turn.
type toolIndexCache struct {
	mu    sync.Mutex
	fp    string
	index *toolindex.Index
}

func (e *Engine) cachedToolIndex(ctx context.Context, specs []llm.ToolSpec) *toolindex.Index {
	fp := toolindex.CatalogFingerprint(specs)
	if e != nil {
		e.embedMu.Lock()
		emb := e.ToolEmbedder
		e.embedMu.Unlock()
		e.toolIdxCache.mu.Lock()
		defer e.toolIdxCache.mu.Unlock()
		if e.toolIdxCache.index != nil && e.toolIdxCache.fp == fp {
			return e.toolIdxCache.index
		}
		idx := toolindex.Build(specs)
		if emb != nil {
			if dense, err := toolindex.BuildDense(ctx, specs, emb); err == nil && dense != nil {
				idx = dense
			}
		}
		e.toolIdxCache.fp = fp
		e.toolIdxCache.index = idx
		return idx
	}
	return toolindex.Build(specs)
}

// prefilterTools deterministically narrows specs to at most limit tools via
// Tool-RAG retrieval (dense+BM25 when ToolEmbedder is set; else lexical).
func prefilterTools(query string, specs []llm.ToolSpec, limit int) (picked []llm.ToolSpec, noMatch bool) {
	picked, noMatch, _, _ = prefilterRetrieve(nil, context.Background(), query, specs, nil, limit)
	return picked, noMatch
}

// prefilterToolsBySystem applies soft source boost for preferred connectors.
func prefilterToolsBySystem(query string, specs []llm.ToolSpec, allowed map[string]bool, totalLimit int) (picked []llm.ToolSpec, noMatch bool) {
	picked, noMatch, _, _ = prefilterRetrieve(nil, context.Background(), query, specs, allowed, totalLimit)
	return picked, noMatch
}

// prefilterRetrieve is the DP-2a entry. When eng is non-nil, uses its embedder
// and catalog cache.
func prefilterRetrieve(eng *Engine, ctx context.Context, query string, specs []llm.ToolSpec, allowed map[string]bool, totalLimit int) (picked []llm.ToolSpec, noMatch, widened bool, mode string) {
	if totalLimit <= 0 || len(specs) == 0 {
		return nil, true, false, ""
	}
	var idx *toolindex.Index
	if eng != nil {
		idx = eng.cachedToolIndex(ctx, specs)
	} else {
		idx = toolindex.Build(specs)
	}
	var emb toolindex.Embedder
	if eng != nil {
		eng.embedMu.Lock()
		emb = eng.ToolEmbedder
		eng.embedMu.Unlock()
	}
	res := idx.Retrieve(ctx, query, toolindex.Options{
		Limit:            totalLimit,
		PreferredSources: allowed,
		WidenIfSparse:    true,
	}, emb)
	if res.Empty {
		return nil, true, res.Widened, res.Mode
	}
	return toolindex.SpecsFromHits(res.Hits), false, res.Widened, res.Mode
}

func cleanQueryTokens(raw map[string]bool) map[string]bool {
	return toolindex.RefineQueryTokens(raw)
}

func tokenSet(s string) map[string]bool {
	return toolindex.Tokens(s)
}
