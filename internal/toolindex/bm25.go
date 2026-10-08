package toolindex

import "math"

// bm25 params (standard Okapi defaults).
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

type bm25Index struct {
	docs    []Document
	df      map[string]int
	avgLen  float64
	docLens []int
	nDocs   int
}

func buildBM25(docs []Document) bm25Index {
	df := make(map[string]int)
	lens := make([]int, len(docs))
	var sum float64
	for i, d := range docs {
		lenTF := 0
		seen := make(map[string]bool, len(d.TermFreq))
		for t, c := range d.TermFreq {
			lenTF += c
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
		lens[i] = lenTF
		sum += float64(lenTF)
	}
	avg := 1.0
	if len(docs) > 0 {
		avg = sum / float64(len(docs))
	}
	return bm25Index{docs: docs, df: df, avgLen: avg, docLens: lens, nDocs: len(docs)}
}

func (idx bm25Index) scores(queryTokens map[string]bool) []float64 {
	out := make([]float64, idx.nDocs)
	if idx.nDocs == 0 {
		return out
	}
	n := float64(idx.nDocs)
	for qt := range queryTokens {
		df := idx.df[qt]
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (n-float64(df)+0.5)/(float64(df)+0.5))
		for i, d := range idx.docs {
			tf := float64(d.TermFreq[qt])
			if tf == 0 {
				continue
			}
			dl := float64(idx.docLens[i])
			denom := tf + bm25K1*(1-bm25B+bm25B*dl/idx.avgLen)
			out[i] += idf * (tf * (bm25K1 + 1) / denom)
		}
	}
	return out
}
