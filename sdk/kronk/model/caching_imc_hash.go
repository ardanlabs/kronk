package model

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// messagePrefixHash hashes a canonical v1 sequence: a domain/version followed
// by tagged little-endian int32 token IDs or tagged 32-byte media identities.
// Text tokens have identical encodings in text and media plans, allowing a
// text checkpoint to seed a media request. No native struct layout is hashed.
// Callers supply increasing prefix lengths, so each input unit is encoded once.
func messagePrefixHash(tokens []llama.Token, units []promptUnit, lengths []int) map[int][sha256.Size]byte {
	hashes := make(map[int][sha256.Size]byte, len(lengths))
	if len(lengths) == 0 {
		return hashes
	}
	h := sha256.New()
	h.Write([]byte("kronk-imc-prefix-v1\x00"))
	var buf [4096]byte
	pos := 0
	for _, end := range lengths {
		n := 0
		for ; pos < end; pos++ {
			if len(buf)-n < 33 {
				h.Write(buf[:n])
				n = 0
			}
			if units != nil && units[pos].isMedia {
				buf[n] = 1
				copy(buf[n+1:], units[pos].media[:])
				n += 33
				continue
			}
			token := llama.Token(0)
			if units != nil {
				token = units[pos].token
			} else {
				token = tokens[pos]
			}
			buf[n] = 0
			binary.LittleEndian.PutUint32(buf[n+1:n+5], uint32(token))
			n += 5
		}
		h.Write(buf[:n])
		var digest [sha256.Size]byte
		h.Sum(digest[:0])
		hashes[end] = digest
	}
	return hashes
}

// updatePrefixHash updates the reject-only lookup metadata while cacheMu
// is held. The session remains reserved until its matching checkpoint commits.
func (s *imcSession) updatePrefixHash() {
	s.prefixLen = 0
	s.prefixHash = [sha256.Size]byte{}
	if s.cache.totalTokensCached == 0 {
		return
	}
	tokens := s.cache.cachedTokens
	var units []promptUnit
	if s.cache.hasMedia {
		units = s.cache.promptPlan.units
	}
	length := len(tokens)
	if units != nil {
		length = len(units)
	}
	if length == 0 {
		return
	}
	s.prefixLen = length
	s.prefixHash = messagePrefixHash(tokens, units, []int{length})[length]
}

// imcCandidateHashes computes only the saved prefix lengths, in a single
// forward pass. Complete resident verification remains mandatory after a hit.
// The caller holds cacheMu, keeping publication stable.
func (m *Model) imcCandidateHashes(tokens []llama.Token, units []promptUnit) map[int][sha256.Size]byte {
	length := len(tokens)
	if units != nil {
		length = len(units)
	}
	lengths := make([]int, 0, len(m.imcSessions))
	for _, s := range m.imcSessions {
		if !s.reserved && s.prefixLen > 0 && s.prefixLen <= length {
			lengths = append(lengths, s.prefixLen)
		}
	}
	slices.Sort(lengths)
	return messagePrefixHash(tokens, units, slices.Compact(lengths))
}

func (s *imcSession) matchesPrefixHash(hashes map[int][sha256.Size]byte) bool {
	digest, ok := hashes[s.prefixLen]
	return ok && digest == s.prefixHash
}

// imcRebuildSession selects an empty or LRU session for rebuilding.
// The caller reserves the result under cacheMu.
func (m *Model) imcRebuildSession() *imcSession {
	var lru *imcSession
	for _, s := range m.imcSessions {
		if s.reserved {
			continue
		}
		if s.cache.totalTokensCached == 0 {
			return s
		}
		if lru == nil || s.lastUsed.Before(lru.lastUsed) {
			lru = s
		}
	}
	return lru
}
