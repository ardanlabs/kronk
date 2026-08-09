package model

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	seedDomainTargetDist uint64 = iota + 1
	seedDomainTargetXTC
	seedDomainTargetAdaptiveP
	seedDomainDraftDist
	seedDomainSpeculative
)

type samplingSeeds struct {
	master          uint32
	generated       bool
	targetDist      uint32
	targetXTC       uint32
	targetAdaptiveP uint32
	draftDist       uint32
	speculative     uint64
}

func resolveSamplingSeeds(seed *uint32) (samplingSeeds, *rand.Rand, error) {
	return resolveSamplingSeedsFrom(seed, cryptorand.Reader)
}

func (m *Model) resolveRequestSamplingSeeds(seed *uint32, seedProvided bool, session *imcSession) (samplingSeeds, *rand.Rand, string, error) {
	seedSource := "provided"
	if !seedProvided {
		seedSource = "configured"
	}
	if !seedProvided && session != nil {
		m.cacheMu.Lock()
		if session.hasSamplingSeed {
			value := session.samplingSeed
			seed = &value
			seedSource = "session"
		}
		m.cacheMu.Unlock()
	}

	seeds, rng, err := resolveSamplingSeeds(seed)
	if err != nil {
		return samplingSeeds{}, nil, "", err
	}
	if seeds.generated {
		seedSource = "generated"
	}

	if session != nil {
		m.cacheMu.Lock()
		session.samplingSeed = seeds.master
		session.hasSamplingSeed = true
		m.cacheMu.Unlock()
	}

	return seeds, rng, seedSource, nil
}

func resolveSamplingSeedsFrom(seed *uint32, entropy io.Reader) (samplingSeeds, *rand.Rand, error) {
	generated := seed == nil
	if seed == nil {
		var buf [4]byte
		if _, err := io.ReadFull(entropy, buf[:]); err != nil {
			return samplingSeeds{}, nil, fmt.Errorf("resolve sampling seed: %w", err)
		}

		value := binary.LittleEndian.Uint32(buf[:])
		seed = &value
	}

	seeds := samplingSeeds{
		master:          *seed,
		generated:       generated,
		targetDist:      deriveNativeSeed(*seed, seedDomainTargetDist),
		targetXTC:       deriveNativeSeed(*seed, seedDomainTargetXTC),
		targetAdaptiveP: deriveNativeSeed(*seed, seedDomainTargetAdaptiveP),
		draftDist:       deriveNativeSeed(*seed, seedDomainDraftDist),
		speculative:     splitMix64(uint64(*seed) + seedDomainSpeculative),
	}

	return seeds, rand.New(rand.NewSource(int64(seeds.speculative))), nil
}

func deriveNativeSeed(master uint32, domain uint64) uint32 {
	value := splitMix64(uint64(master) + domain)
	for seed := uint32(value); ; seed = uint32(value) {
		if seed != llama.DefaultSeed {
			return seed
		}
		value = splitMix64(value)
	}
}

func splitMix64(value uint64) uint64 {
	value += 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}
