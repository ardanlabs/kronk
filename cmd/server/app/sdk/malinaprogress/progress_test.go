package malinaprogress

import "testing"

func TestBrokerBroadcastsLatestUpdate(t *testing.T) {
	broker := New()
	fast, unsubscribeFast := broker.Subscribe()
	defer unsubscribeFast()
	slow, unsubscribeSlow := broker.Subscribe()
	defer unsubscribeSlow()

	broker.Publish(1, 4, 0.5)

	if got := <-fast; got.Step != 1 || got.Steps != 4 || got.SecondsPerStep != 0.5 {
		t.Errorf("fast update: got %+v, want step 1 of 4 at 0.5 seconds", got)
	}

	broker.Publish(2, 4, 0.4)
	broker.Publish(3, 4, 0.3)

	if got := <-slow; got.Step != 3 || got.Steps != 4 || got.SecondsPerStep != 0.3 {
		t.Errorf("slow update: got %+v, want latest step 3 of 4 at 0.3 seconds", got)
	}
}

func TestBrokerUnsubscribe(t *testing.T) {
	broker := New()
	updates, unsubscribe := broker.Subscribe()
	unsubscribe()

	broker.Publish(1, 1, 0.1)

	select {
	case got := <-updates:
		t.Fatalf("update after unsubscribe: got %+v", got)
	default:
	}
}
