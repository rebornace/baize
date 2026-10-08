package toolindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/rebornace/baize/internal/llm"
)

func errDimMismatch(got, want int) error {
	return fmt.Errorf("toolindex: embed returned %d vectors, want %d", got, want)
}

// DefaultSourceBoost multiplies a document's fused score when its Source is in
// the preferred set. Values >1 prefer routed systems without hiding others.
const DefaultSourceBoost = 1.35

const rrfK = 60

// Mode names reported on Result for observability.
const (
	ModeDenseHybrid = "dense_bm25"
	ModeLexical     = "bm25_sparse"
)

// Options control hybrid retrieval.
type Options struct {
	Limit            int
	PreferredSources map[string]bool
	SourceBoost      float64
	WidenIfSparse    bool
}

// Hit is one retrieved tool with debug scores.
type Hit struct {
	Spec          llm.ToolSpec
	Score         float64
	BM25          float64
	Cosine        float64 // sparse TF-IDF and/or dense cosine (max used in RRF)
	Dense         float64
	SourceBoosted bool
}

// Result is the Top-K retrieval outcome.
type Result struct {
	Hits    []Hit
	Empty   bool
	Widened bool
	Mode    string
}

// Index is a snapshot of the tool catalog for retrieve calls.
type Index struct {
	specs  []llm.ToolSpec
	docs   []Document
	bm25   bm25Index
	sparse vectorIndex
	dense  [][]float32 // nil ⇒ lexical-only
	mode   string
}

// Build constructs a lexical index (BM25 + sparse TF-IDF). Use BuildDense when
// an Embedder is configured.
func Build(specs []llm.ToolSpec) *Index {
	docs := make([]Document, len(specs))
	for i, s := range specs {
		docs[i] = BuildDocument(s)
	}
	return &Index{
		specs:  specs,
		docs:   docs,
		bm25:   buildBM25(docs),
		sparse: buildVectors(docs),
		mode:   ModeLexical,
	}
}

// BuildDense embeds each tool document then builds a hybrid index. On embed
// failure the caller should fall back to Build.
func BuildDense(ctx context.Context, specs []llm.ToolSpec, emb Embedder) (*Index, error) {
	idx := Build(specs)
	if emb == nil || len(specs) == 0 {
		return idx, nil
	}
	texts := make([]string, len(idx.docs))
	for i, d := range idx.docs {
		texts[i] = d.Text
	}
	vecs, err := emb.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(specs) {
		return nil, errDimMismatch(len(vecs), len(specs))
	}
	idx.dense = vecs
	idx.mode = ModeDenseHybrid
	return idx, nil
}

// CatalogFingerprint is a stable hash of the searchable tool surface so callers
// can cache an Index across turns until the catalog changes.
func CatalogFingerprint(specs []llm.ToolSpec) string {
	h := sha256.New()
	for _, s := range specs {
		h.Write([]byte(s.Name))
		h.Write([]byte{0})
		h.Write([]byte(s.Source))
		h.Write([]byte{0})
		h.Write([]byte(s.Method))
		h.Write([]byte{0})
		h.Write([]byte(s.Path))
		h.Write([]byte{0})
		h.Write([]byte(firstLine(s.Description)))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

type rankRow struct {
	i       int
	bm25    float64
	cos     float64
	dense   float64
	fused   float64
	boosted bool
}

// Retrieve ranks tools for query and returns at most opts.Limit hits.
func (idx *Index) Retrieve(ctx context.Context, query string, opts Options, emb Embedder) Result {
	if idx == nil || opts.Limit <= 0 || len(idx.specs) == 0 {
		return Result{Empty: true, Mode: ModeLexical}
	}
	// Lexical channel: drop fillers only (no verb synonym expand). Dense
	// embeds the raw query string so cross-lingual matching stays model-side.
	qToks := RefineQueryTokens(Tokens(query))
	if len(qToks) == 0 && (idx.dense == nil || emb == nil) {
		return Result{Empty: true, Mode: idx.mode}
	}

	var qDense []float32
	if len(idx.dense) > 0 && emb != nil {
		vecs, err := emb.Embed(ctx, []string{query})
		if err == nil && len(vecs) == 1 {
			qDense = vecs[0]
		}
	}

	hits, empty := idx.rank(qToks, qDense, opts)
	widened := false
	if opts.WidenIfSparse && !empty && len(opts.PreferredSources) > 0 {
		if len(hits) < opts.Limit/2 {
			soft := opts
			soft.PreferredSources = nil
			soft.WidenIfSparse = false
			alt, altEmpty := idx.rank(qToks, qDense, soft)
			if !altEmpty {
				hits = alt
				widened = true
				empty = false
			}
		}
	}
	if empty || len(hits) == 0 {
		return Result{Empty: true, Widened: widened, Mode: idx.mode}
	}
	if len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}
	return Result{Hits: hits, Empty: false, Widened: widened, Mode: idx.mode}
}

// RetrieveLexical is Retrieve without a query embedder (BM25 + sparse only).
func (idx *Index) RetrieveLexical(query string, opts Options) Result {
	return idx.Retrieve(context.Background(), query, opts, nil)
}

func (idx *Index) rank(qToks map[string]bool, qDense []float32, opts Options) ([]Hit, bool) {
	bm25s := idx.bm25.scores(qToks)
	sparse := idx.sparse.scores(qToks)
	denseScores := make([]float64, len(idx.specs))
	hasDense := len(qDense) > 0 && len(idx.dense) == len(idx.specs)
	if hasDense {
		for i := range idx.specs {
			denseScores[i] = cosineDense(qDense, idx.dense[i])
		}
	}

	boost := opts.SourceBoost
	if boost <= 0 {
		boost = DefaultSourceBoost
	}

	rrf := make([]float64, len(idx.specs))
	addRRF := func(scores []float64, weight float64) {
		order := argsortDesc(scores)
		for rank, i := range order {
			if scores[i] <= 0 {
				break
			}
			rrf[i] += weight / float64(rrfK+rank+1)
		}
	}
	addRRF(bm25s, 1.0)
	if hasDense {
		// Dense is the primary channel (ToolBench-style); BM25 is a light hybrid.
		addRRF(denseScores, 2.0)
	} else {
		addRRF(sparse, 1.0)
	}

	rows := make([]rankRow, 0, len(idx.specs))
	for i := range idx.specs {
		cos := sparse[i]
		if hasDense && denseScores[i] > cos {
			cos = denseScores[i]
		}
		if rrf[i] <= 0 && bm25s[i] <= 0 && cos <= 0 {
			continue
		}
		fused := rrf[i] + 1e-6*bm25s[i]
		if hasDense {
			fused += 1e-4 * denseScores[i]
		}
		boosted := false
		src := idx.specs[i].Source
		if len(opts.PreferredSources) > 0 {
			if src == "" || opts.PreferredSources[src] {
				fused *= boost
				boosted = true
			}
		}
		rows = append(rows, rankRow{
			i: i, bm25: bm25s[i], cos: cos, dense: denseScores[i],
			fused: fused, boosted: boosted,
		})
	}
	if len(rows) == 0 {
		return nil, true
	}
	sort.SliceStable(rows, func(a, b int) bool {
		if rows[a].fused != rows[b].fused {
			return rows[a].fused > rows[b].fused
		}
		return rows[a].i < rows[b].i
	})

	picked := pickWithSoftQuota(rows, idx.specs, opts.PreferredSources, opts.Limit)
	hits := make([]Hit, 0, len(picked))
	for _, r := range picked {
		hits = append(hits, Hit{
			Spec:          idx.specs[r.i],
			Score:         r.fused,
			BM25:          r.bm25,
			Cosine:        r.cos,
			Dense:         r.dense,
			SourceBoosted: r.boosted,
		})
	}
	return hits, false
}

func pickWithSoftQuota(rows []rankRow, specs []llm.ToolSpec, preferred map[string]bool, limit int) []rankRow {
	if limit <= 0 {
		return nil
	}
	if len(preferred) == 0 {
		if len(rows) > limit {
			return rows[:limit]
		}
		return rows
	}

	bySrc := make(map[string][]rankRow)
	var other []rankRow
	srcOrder := make([]string, 0)
	seenSrc := map[string]bool{}
	for _, r := range rows {
		src := specs[r.i].Source
		if src == "" || preferred[src] {
			if !seenSrc[src] {
				seenSrc[src] = true
				srcOrder = append(srcOrder, src)
			}
			bySrc[src] = append(bySrc[src], r)
		} else {
			other = append(other, r)
		}
	}
	sort.SliceStable(srcOrder, func(a, b int) bool {
		if srcOrder[a] == "" {
			return true
		}
		if srcOrder[b] == "" {
			return false
		}
		return srcOrder[a] < srcOrder[b]
	})

	out := make([]rankRow, 0, limit)
	used := map[int]bool{}
	for len(out) < limit {
		progressed := false
		for _, src := range srcOrder {
			q := bySrc[src]
			if len(q) == 0 {
				continue
			}
			r := q[0]
			bySrc[src] = q[1:]
			if used[r.i] {
				continue
			}
			out = append(out, r)
			used[r.i] = true
			progressed = true
			if len(out) == limit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	for _, r := range other {
		if len(out) >= limit {
			break
		}
		if used[r.i] {
			continue
		}
		out = append(out, r)
		used[r.i] = true
	}
	if len(out) < limit {
		for _, r := range rows {
			if len(out) >= limit {
				break
			}
			if used[r.i] {
				continue
			}
			out = append(out, r)
			used[r.i] = true
		}
	}
	return out
}

func argsortDesc(scores []float64) []int {
	idx := make([]int, len(scores))
	for i := range scores {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if scores[idx[a]] != scores[idx[b]] {
			return scores[idx[a]] > scores[idx[b]]
		}
		return idx[a] < idx[b]
	})
	return idx
}

// SpecsFromHits unwraps Hit specs in rank order.
func SpecsFromHits(hits []Hit) []llm.ToolSpec {
	out := make([]llm.ToolSpec, len(hits))
	for i, h := range hits {
		out[i] = h.Spec
	}
	return out
}

