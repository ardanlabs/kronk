package kronk

// ModelInfo represents the model's card information.
type ModelInfo struct {
	Desc        string
	Size        uint64
	HasEncoder  bool
	HasDecoder  bool
	IsRecurrent bool
	IsHybrid    bool
	Metadata    map[string]string
}

// ChatMessage represent input for chat and vision models.
type ChatMessage struct {
	Role    string
	Content string
}

type Tokens struct {
	Input   int
	Output  int
	Context int
}

// ChatResponse represents output for chat and vision models.
type ChatResponse struct {
	Response string
	Err      error
	Tokens   Tokens
}

// RankingDocument represents input for reranking.
type RankingDocument struct {
	Document  string
	Embedding []float64
}

// Ranking represents output for reranking.
type Ranking struct {
	Document string
	Score    float64
}
