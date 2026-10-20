package testlib

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// CheckEmbeddings validates the Nomic fixture's normalized vectors and indexed
// response envelope. Input identity is checked against isolated requests below.
func CheckEmbeddings(resp model.EmbedReponse, id string, count, dimensions int) error {
	if resp.Model != id || resp.Object != "list" || resp.Created <= 0 {
		return fmt.Errorf("invalid embedding envelope: model=%q object=%q created=%d", resp.Model, resp.Object, resp.Created)
	}
	if len(resp.Data) != count {
		return fmt.Errorf("embedding count: got %d, want %d", len(resp.Data), count)
	}
	if resp.Usage.PromptTokens <= 0 || resp.Usage.TotalTokens != resp.Usage.PromptTokens {
		return fmt.Errorf("invalid embedding usage: %+v", resp.Usage)
	}
	for i, data := range resp.Data {
		if data.Index != i || data.Object != "embedding" || len(data.Embedding) != dimensions {
			return fmt.Errorf("embedding[%d]: index=%d object=%q dimensions=%d, want %d", i, data.Index, data.Object, len(data.Embedding), dimensions)
		}
		var squaredNorm float64
		for j, value := range data.Embedding {
			v := float64(value)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("embedding[%d][%d]: nonfinite value %v", i, j, value)
			}
			squaredNorm += v * v
		}
		if math.Abs(math.Sqrt(squaredNorm)-1) > 1e-4 {
			return fmt.Errorf("embedding[%d]: L2 norm %g, want 1 within 1e-4", i, math.Sqrt(squaredNorm))
		}
	}
	return nil
}

// CompareEmbeddingVectors requires cosine similarity of at least 0.999, allowing
// single/batched numerical variation across inference backends. If other input
// references are supplied, got must be strictly closer to want than to any other.
func CompareEmbeddingVectors(want, got []float32, others ...[]float32) error {
	var wantSimilarity float64
	for j, reference := range append([][]float32{want}, others...) {
		if len(got) != len(reference) {
			return fmt.Errorf("reference[%d] vector dimensions: got %d, want %d", j, len(got), len(reference))
		}
		var dot, gotNorm, referenceNorm float64
		for i, value := range got {
			a, b := float64(value), float64(reference[i])
			if math.IsNaN(a) || math.IsInf(a, 0) || math.IsNaN(b) || math.IsInf(b, 0) {
				return fmt.Errorf("reference[%d] vector[%d]: nonfinite value/reference %g / %g", j, i, a, b)
			}
			dot += a * b
			gotNorm += a * a
			referenceNorm += b * b
		}
		if gotNorm == 0 || referenceNorm == 0 {
			return fmt.Errorf("reference[%d]: cosine similarity undefined for zero vector", j)
		}
		similarity := dot / math.Sqrt(gotNorm*referenceNorm)
		if j == 0 {
			if similarity < 0.999 {
				return fmt.Errorf("vector cosine similarity: got %g, want at least 0.999", similarity)
			}
			wantSimilarity = similarity
			continue
		}
		if similarity >= wantSimilarity {
			return fmt.Errorf("vector identity: other input[%d] similarity %g, want less than intended input similarity %g", j-1, similarity, wantSimilarity)
		}
	}
	return nil
}

// TestEmbeddingIntegrity compares reordered batches and concurrent requests with
// single-input references using the existing Nomic fixture, without reloading it.
func TestEmbeddingIntegrity(t *testing.T, id string, embed func(context.Context, model.D) (model.EmbedReponse, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), TestDuration)
	defer cancel()
	inputs := []string{
		"The red fox sleeps beneath a cedar tree.",
		"Database transactions roll back partial writes after failures.",
		"Paris is the capital of France; Tokyo is the capital of Japan.",
	}
	references := make([][]float32, len(inputs))
	for i, input := range inputs {
		resp, err := embed(ctx, model.D{"input": input})
		if err != nil {
			t.Fatal(err)
		}
		if err := CheckEmbeddings(resp, id, 1, 768); err != nil {
			t.Fatal(err)
		}
		references[i] = resp.Data[0].Embedding
	}
	if CompareEmbeddingVectors(references[0], references[1]) == nil {
		t.Fatal("asymmetric input controls produced indistinguishable embeddings")
	}
	check := func(order []int, resp model.EmbedReponse, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if err := CheckEmbeddings(resp, id, len(order), 768); err != nil {
			t.Fatal(err)
		}
		for i, index := range order {
			others := slices.Concat(references[:index], references[index+1:])
			if err := CompareEmbeddingVectors(references[index], resp.Data[i].Embedding, others...); err != nil {
				t.Fatalf("input[%d] at output[%d]: %v", index, i, err)
			}
		}
	}
	request := func(order []int) model.D {
		batch := make([]string, len(order))
		for i, index := range order {
			batch[i] = inputs[index]
		}
		return model.D{"input": batch}
	}
	order := []int{2, 0, 1}
	resp, err := embed(ctx, request(order))
	check(order, resp, err)

	orders := [][]int{{0, 1}, {2}, {1, 0}}
	responses := make([]model.EmbedReponse, len(orders))
	errs := make([]error, len(orders))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, order := range orders {
		wg.Go(func() {
			<-start
			responses[i], errs[i] = embed(ctx, request(order))
		})
	}
	close(start)
	wg.Wait()
	for i, order := range orders {
		check(order, responses[i], errs[i])
	}
	t.Log("isolated, reordered-batch and concurrent embedding identities match")
}

// CheckRerank validates exact result counts, document identity, unique source
// indices, finite sorted scores and usage for a requested top_n/document mode.
func CheckRerank(resp model.RerankResponse, id string, documents []string, topN int, returnDocuments bool) error {
	if resp.Model != id || resp.Object != "list" || resp.Created <= 0 {
		return fmt.Errorf("invalid rerank envelope: model=%q object=%q created=%d", resp.Model, resp.Object, resp.Created)
	}
	count := len(documents)
	if topN > 0 {
		count = min(count, topN)
	}
	if len(resp.Data) != count {
		return fmt.Errorf("rerank count: got %d, want %d", len(resp.Data), count)
	}
	if resp.Usage.PromptTokens <= 0 || resp.Usage.TotalTokens != resp.Usage.PromptTokens {
		return fmt.Errorf("invalid rerank usage: %+v", resp.Usage)
	}
	seen := make(map[int]bool)
	for i, result := range resp.Data {
		if result.Index < 0 || result.Index >= len(documents) || seen[result.Index] {
			return fmt.Errorf("rerank[%d]: invalid/repeated source index %d", i, result.Index)
		}
		seen[result.Index] = true
		score := float64(result.RelevanceScore)
		if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
			return fmt.Errorf("rerank[%d]: score %g outside finite [0,1]", i, score)
		}
		if i > 0 && result.RelevanceScore > resp.Data[i-1].RelevanceScore {
			return fmt.Errorf("rerank[%d]: scores are not descending", i)
		}
		want := ""
		if returnDocuments {
			want = documents[result.Index]
		}
		if result.Document != want {
			return fmt.Errorf("rerank[%d]: document %q, want source[%d]=%q", i, result.Document, result.Index, want)
		}
	}
	return nil
}

// TestRerankIntegrity checks multi-document and concurrent requests against
// independent one-document scores, with different queries and source ordering.
func TestRerankIntegrity(t *testing.T, id string, rerank func(context.Context, model.D) (model.RerankResponse, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), TestDuration)
	defer cancel()
	fixtures := []struct {
		query     string
		documents []string
		winner    int
	}{
		{"What is the capital of France?", []string{"Berlin is the capital of Germany.", "Bananas grow in tropical climates.", "Paris is the capital and largest city of France."}, 2},
		{"What is the capital of Germany?", []string{"Paris is the capital of France.", "Berlin is the capital and largest city of Germany.", "Rain falls from clouds.", "Tokyo is the capital of Japan."}, 1},
	}
	references := make([][]model.RerankResult, len(fixtures))
	for i, fixture := range fixtures {
		for j, document := range fixture.documents {
			resp, err := rerank(ctx, model.D{"query": fixture.query, "documents": []string{document}, "return_documents": true})
			if err != nil {
				t.Fatal(err)
			}
			if err := CheckRerank(resp, id, []string{document}, 0, true); err != nil {
				t.Fatal(err)
			}
			result := resp.Data[0]
			result.Index = j
			references[i] = append(references[i], result)
		}
		slices.SortFunc(references[i], func(a, b model.RerankResult) int { return cmp.Compare(b.RelevanceScore, a.RelevanceScore) })
		if references[i][0].Index != fixture.winner {
			t.Fatalf("query[%d] isolated winner: got %d, want %d", i, references[i][0].Index, fixture.winner)
		}
	}
	for _, mode := range []struct {
		topN      int
		documents bool
	}{{0, false}, {2, true}, {99, false}} {
		responses := make([]model.RerankResponse, len(fixtures))
		errs := make([]error, len(fixtures))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, fixture := range fixtures {
			wg.Go(func() {
				<-start
				request := model.D{"query": fixture.query, "documents": fixture.documents, "return_documents": mode.documents}
				if mode.topN > 0 {
					request["top_n"] = mode.topN
				}
				responses[i], errs[i] = rerank(ctx, request)
			})
		}
		close(start)
		wg.Wait()
		for i, fixture := range fixtures {
			if errs[i] != nil {
				t.Fatal(errs[i])
			}
			if err := CheckRerank(responses[i], id, fixture.documents, mode.topN, mode.documents); err != nil {
				t.Fatal(err)
			}
			for j, result := range responses[i].Data {
				want := references[i][j]
				if result.Index != want.Index || math.Abs(float64(result.RelevanceScore-want.RelevanceScore)) > 1e-3 {
					t.Fatalf("query[%d] rank[%d]: got index=%d score=%g, want index=%d score=%g within 1e-3", i, j, result.Index, result.RelevanceScore, want.Index, want.RelevanceScore)
				}
			}
		}
	}
	t.Log("concurrent rerank indices/scores match isolated documents for default, top_n and omitted-document modes")
}

// CheckDecisionProbabilities validates the requested labels, rounded probability
// distribution and confidence returned by a choice or score question.
func CheckDecisionProbabilities(answer model.DecisionAnswer, labels []string) error {
	if len(answer.Probabilities) != len(labels) {
		return fmt.Errorf("probabilities: got %d labels, want %d", len(answer.Probabilities), len(labels))
	}
	var sum float64
	for _, label := range labels {
		value, exists := answer.Probabilities[label]
		if !exists || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("probability %q: got %g (exists=%t), want finite [0,1]", label, value, exists)
		}
		sum += value
	}
	// Each probability is rounded to four decimal places by the decision API.
	if math.Abs(sum-1) > float64(len(labels))*0.00005+1e-9 {
		return fmt.Errorf("probability sum: got %g, want 1 within rounding tolerance", sum)
	}
	if value := answer.Confidence; math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("confidence: got %g, want finite [0,1]", value)
	}
	if answer.Type == model.DecisionQuestionTypeChoice && !slices.Contains(labels, answer.Choice) {
		return fmt.Errorf("choice %q is not a requested label", answer.Choice)
	}
	return nil
}
