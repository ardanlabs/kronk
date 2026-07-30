package model

// supportsBatchSeq reports whether the model uses the multi-sequence batch
// runtime. All embedding and reranking models use it by default. This selection
// point is retained so a model proven incompatible can be routed to the context
// pool fallback without changing either runtime.
func supportsBatchSeq(mi ModelInfo) bool {
	// Return false here for any model proven incompatible with the batch
	// sequence engine so it uses the context pool fallback instead.
	return mi.IsEmbedModel || mi.IsRerankModel
}
