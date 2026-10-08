// Package toolindex implements Tool-RAG style candidate retrieval for large
// tool catalogs: each tool is indexed as a document (name, description, HTTP
// method/path, parameter names).
//
// Primary path: dense Embedder cosine (OpenAI-compatible /embeddings or any
// multilingual model) fused with BM25 via RRF — same idea as ToolBench.
// Fallback without an Embedder: BM25 + sparse TF-IDF (lexical only).
// System routing is a soft source boost, never a hard gate.
package toolindex
