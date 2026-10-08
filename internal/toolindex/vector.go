package toolindex

import "math"

// Sparse TF-IDF vectors over the same token space as BM25. Used only when no
// dense Embedder is configured; with Embedder, dense cosine replaces this
// channel in RRF (BM25 remains as the lexical hybrid leg).
type sparseVec map[string]float64

type vectorIndex struct {
	vecs []sparseVec
	idf  map[string]float64
}

func buildVectors(docs []Document) vectorIndex {
	df := make(map[string]int)
	for _, d := range docs {
		for t := range d.TermFreq {
			df[t]++
		}
	}
	n := float64(len(docs))
	idf := make(map[string]float64, len(df))
	for t, c := range df {
		idf[t] = math.Log(1 + n/float64(c))
	}
	vecs := make([]sparseVec, len(docs))
	for i, d := range docs {
		v := make(sparseVec, len(d.TermFreq))
		var norm float64
		for t, c := range d.TermFreq {
			w := float64(c) * idf[t]
			v[t] = w
			norm += w * w
		}
		if norm > 0 {
			inv := 1 / math.Sqrt(norm)
			for t := range v {
				v[t] *= inv
			}
		}
		vecs[i] = v
	}
	return vectorIndex{vecs: vecs, idf: idf}
}

func (idx vectorIndex) queryVec(queryTokens map[string]bool) sparseVec {
	v := make(sparseVec, len(queryTokens))
	var norm float64
	for t := range queryTokens {
		w := idx.idf[t]
		if w == 0 {
			continue
		}
		v[t] = w
		norm += w * w
	}
	if norm > 0 {
		inv := 1 / math.Sqrt(norm)
		for t := range v {
			v[t] *= inv
		}
	}
	return v
}

func (idx vectorIndex) scores(queryTokens map[string]bool) []float64 {
	out := make([]float64, len(idx.vecs))
	qv := idx.queryVec(queryTokens)
	if len(qv) == 0 {
		return out
	}
	for i, dv := range idx.vecs {
		var dot float64
		// Iterate the smaller map.
		a, b := qv, dv
		if len(dv) < len(qv) {
			a, b = dv, qv
		}
		for t, wa := range a {
			if wb, ok := b[t]; ok {
				dot += wa * wb
			}
		}
		out[i] = dot
	}
	return out
}
