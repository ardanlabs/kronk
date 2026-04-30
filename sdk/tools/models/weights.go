package models

// WeightBreakdown provides per-category weight size information. Field
// shape mirrors gguf.WeightBreakdown so the two are interchangeable;
// this models-side type lets the package keep its public API stable
// without leaking the gguf import in struct fields.
type WeightBreakdown struct {
	TotalBytes         int64
	AlwaysActiveBytes  int64
	ExpertBytesTotal   int64
	ExpertBytesByLayer []int64
}
