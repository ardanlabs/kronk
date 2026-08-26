package model

import (
	"errors"
	"slices"
	"testing"
)

func TestDecodeMTPMirrorChunks(t *testing.T) {
	t.Run("sync failure disables MTP and continues target", func(t *testing.T) {
		var targetChunks [][2]int
		var syncChunks [][2]int
		var disabled []error
		syncErr := errors.New("sync")

		err := decodeMTPMirrorChunks(5, 2, func(start, end int) error {
			targetChunks = append(targetChunks, [2]int{start, end})
			return nil
		}, func(start, end int) error {
			syncChunks = append(syncChunks, [2]int{start, end})
			if start == 0 {
				return nil
			}
			return syncErr
		}, func(err error) error {
			disabled = append(disabled, err)
			return nil
		})
		if err != nil {
			t.Fatalf("decodeMTPMirrorChunks() error = %v, want nil", err)
		}
		if want := [][2]int{{0, 2}, {2, 4}, {4, 5}}; !slices.Equal(targetChunks, want) {
			t.Errorf("target chunks = %v, want %v", targetChunks, want)
		}
		if want := [][2]int{{0, 2}, {2, 4}}; !slices.Equal(syncChunks, want) {
			t.Errorf("sync chunks = %v, want %v", syncChunks, want)
		}
		if len(disabled) != 1 || !errors.Is(disabled[0], syncErr) {
			t.Errorf("disabled errors = %v, want [%v]", disabled, syncErr)
		}
	})

	t.Run("target failure remains fatal", func(t *testing.T) {
		targetErr := errors.New("target")
		disableCalls := 0

		err := decodeMTPMirrorChunks(5, 2, func(start, _ int) error {
			if start == 2 {
				return targetErr
			}
			return nil
		}, func(_, _ int) error {
			return nil
		}, func(error) error {
			disableCalls++
			return nil
		})
		if !errors.Is(err, targetErr) {
			t.Fatalf("decodeMTPMirrorChunks() error = %v, want %v", err, targetErr)
		}
		if disableCalls != 0 {
			t.Errorf("disable calls = %d, want 0", disableCalls)
		}
	})

	t.Run("draft cleanup failure remains fatal", func(t *testing.T) {
		syncErr := errors.New("sync")
		disableErr := errors.New("disable")

		err := decodeMTPMirrorChunks(5, 2, func(_, _ int) error {
			return nil
		}, func(_, _ int) error {
			return syncErr
		}, func(error) error {
			return disableErr
		})
		if !errors.Is(err, disableErr) {
			t.Fatalf("decodeMTPMirrorChunks() error = %v, want %v", err, disableErr)
		}
	})
}
