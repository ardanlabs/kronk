package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestMessagePrefixHash(t *testing.T) {
	lengths := []int{1, 813, 819, 821, 1640}
	for _, media := range []bool{false, true} {
		t.Run(fmt.Sprintf("media=%v", media), func(t *testing.T) {
			tokens := make([]llama.Token, 1640)
			units := make([]promptUnit, len(tokens))
			encoded := []byte("kronk-imc-prefix-v1\x00")
			want := make(map[int][32]byte)
			for i := range tokens {
				// Assemble expected wire bytes independently of the encoder.
				tokens[i] = -1
				wire := []byte{0, 255, 255, 255, 255}
				if i%2 != 0 {
					tokens[i] = 0x01020304
					wire = []byte{0, 4, 3, 2, 1}
				}
				units[i] = promptUnit{token: tokens[i]}
				if media && i == 818 {
					units[i] = promptUnit{isMedia: true, media: [32]byte{7, 9}}
					wire = make([]byte, 33)
					wire[0], wire[1], wire[2] = 1, 7, 9
				}
				encoded = append(encoded, wire...)
				if slices.Contains(lengths, i+1) {
					want[i+1] = sha256.Sum256(encoded)
				}
			}
			got := messagePrefixHash(nil, units, lengths)
			for _, n := range lengths {
				if got[n] != want[n] {
					t.Errorf("prefix %d: got %x, want %x", n, got[n], want[n])
				}
			}
			if !media {
				text := messagePrefixHash(tokens, nil, lengths)
				for _, n := range lengths {
					if text[n] != want[n] {
						t.Errorf("text prefix %d: got %x, want %x", n, text[n], want[n])
					}
				}
				// Unsorted, duplicate candidate lengths must still discover
				// the longest complete prefix across hashing buffer boundaries.
				m := &Model{cfg: Config{PtrCacheMinTokens: new(1)}, log: applog.DiscardLogger}
				for i, n := range []int{819, 1, 1640, 813, 819} {
					m.imcSessions = append(m.imcSessions, &imcSession{id: i, cache: &imcPrefix{cachedTokens: slices.Clone(tokens[:n]), totalTokensCached: n}, kvState: populatedTestSessionStore(t)})
				}
				publishTestIMCPrefixes(m)
				stable := append(slices.Clone(tokens), 5)
				actual := append(slices.Clone(stable), 6)
				result := m.processIMCTokenPlan(context.Background(), D{}, actual, stable, nil, time.Now())
				if result.err != nil || result.imcSession != m.imcSessions[2] || result.cacheIdx != 1640 || result.imcMatchKind != "append" || !slices.Equal(result.imcNewCacheTokens, []llama.Token{5}) {
					t.Fatalf("mixed-length selection: got session=%p prefix=%d kind=%q extension=%v err=%v", result.imcSession, result.cacheIdx, result.imcMatchKind, result.imcNewCacheTokens, result.err)
				}
			}
		})
	}
}

// A forced digest collision must not authorize a restore. Exercise the real
// planners, not just the digest helper, for both text and media identities.
func TestIMCPrefixHashCollision(t *testing.T) {
	for _, name := range []string{"text", "media-identity", "media-bytes"} {
		t.Run(name, func(t *testing.T) {
			media := name != "text"
			m := &Model{cfg: Config{PtrCacheMinTokens: new(1)}, log: applog.DiscardLogger}
			s := &imcSession{id: 0, cache: &imcPrefix{cachedTokens: []llama.Token{1, 9}, totalTokensCached: 2}, kvState: populatedTestSessionStore(t)}
			m.imcSessions = []*imcSession{s}
			stable := []llama.Token{1, 2}
			actual := []llama.Token{1, 2, 3}
			plan := mediaPlan(promptUnit{token: 1}, promptUnit{isMedia: true, media: [32]byte{2}})
			if media {
				s.cache.hasMedia = true
				s.cache.promptPlan = mediaPlan(promptUnit{token: 1}, promptUnit{isMedia: true, media: [32]byte{9}})
				if name == "media-bytes" {
					// Even a collision in the per-media digest cannot restore
					// another payload's native checkpoint.
					s.cache.promptPlan.units[1].media = plan.units[1].media
					s.cache.promptPlan.mediaBytes = [][]byte{[]byte("old image")}
					plan.mediaBytes = [][]byte{[]byte("new image")}
				}
				s.cache.mediaKVCounts = []int{1}
				s.cache.samplerPromptTokens = []llama.Token{1, 7}
			}
			publishTestIMCPrefixes(m)
			var hashes map[int][32]byte
			if media {
				hashes = m.imcCandidateHashes(nil, plan.units)
			} else {
				hashes = m.imcCandidateHashes(stable, nil)
			}
			s.prefixHash = hashes[2]
			if !s.matchesPrefixHash(hashes) {
				t.Fatal("collision fixture did not match lookup digest")
			}
			d := D{"messages": []D{{"role": "user", "content": "hello"}}}
			s.cache.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))
			var result cacheResult
			if media {
				full := mediaPlan(append(slices.Clone(plan.units), promptUnit{token: 3})...)
				full.mediaBytes = plan.mediaBytes
				result = m.processIMCMediaPlans(context.Background(), d, d, full, plan, []llama.Token{3}, nil, time.Now())
			} else {
				result = m.processIMCTokenPlan(context.Background(), d, actual, stable, nil, time.Now())
			}
			if result.err != nil || result.imcMatchKind != "rebuild" || result.cacheIdx != 0 || !result.imcClearSeq {
				t.Fatalf("collision plan: got kind=%q reused=%d clear=%v err=%v, want rebuild without restore", result.imcMatchKind, result.cacheIdx, result.imcClearSeq, result.err)
			}
		})
	}
}

func BenchmarkIMCPrefixLookup(b *testing.B) {
	for _, size := range []int{1024, 32768} {
		for _, capacity := range []int{1, 8, 64, 256} {
			for _, scenario := range []string{"late-miss", "early-miss", "shared-hit", "mixed-length"} {
				b.Run(fmt.Sprintf("tokens=%d/entries=%d/%s", size, capacity, scenario), func(b *testing.B) {
					input := make([]llama.Token, size+1)
					for i := range input {
						input[i] = llama.Token(i%30000 + 1)
					}
					m := &Model{}
					for i := range capacity {
						n := size
						if scenario == "mixed-length" {
							n -= i % min(size, 64)
						}
						tokens := slices.Clone(input[:n])
						switch scenario {
						case "early-miss":
							tokens[0] = -1
						case "late-miss", "mixed-length":
							tokens[n-1] = -1
						}
						s := &imcSession{id: i, cache: &imcPrefix{cachedTokens: tokens, totalTokensCached: n}}
						m.imcSessions = append(m.imcSessions, s)
						s.updatePrefixHash()
					}
					b.Run("prefix-scan", func(b *testing.B) {
						b.ReportAllocs()
						hits := 0
						for b.Loop() {
							for _, s := range m.imcSessions {
								if tokensHavePrefix(input, s.cache.cachedTokens) {
									hits++
								}
							}
						}
						if scenario == "shared-hit" && hits != b.N*capacity || scenario != "shared-hit" && hits != 0 {
							b.Fatalf("incorrect baseline hits: %d", hits)
						}
					})
					b.Run("hash-verified", func(b *testing.B) {
						b.ReportAllocs()
						hits := 0
						for b.Loop() {
							hashes := m.imcCandidateHashes(input, nil)
							for _, s := range m.imcSessions {
								if s.matchesPrefixHash(hashes) && tokensHavePrefix(input, s.cache.cachedTokens) {
									hits++
								}
							}
						}
						if scenario == "shared-hit" && hits != b.N*capacity || scenario != "shared-hit" && hits != 0 {
							b.Fatalf("incorrect hash lookup hits: %d", hits)
						}
					})
				})
			}
		}
	}
}
