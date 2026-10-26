package ui

import (
	"errors"
	"testing"
	"time"
)

func TestRetainedRendererRetryBoundAndStableReset(t *testing.T) {
	fail := false
	var g *testGPU
	h, _, frame := gpuHost(t, func() (gpuRenderer, error) {
		if fail {
			return nil, errors.New("fixture unavailable")
		}
		g = &testGPU{}
		return g, nil
	})
	defer h.detach()
	frame()
	g.fail = true
	fail = true
	frame()
	for i := 0; i < 8; i++ {
		h.retryAt = time.Now()
		frame()
		if h.backoff < time.Second || h.backoff > 30*time.Second {
			t.Fatal("unbounded backoff", h.backoff)
		}
	}
	if h.backoff != 30*time.Second {
		t.Fatal("backoff did not reach cap")
	}
	fail = false
	h.retryAt = time.Now()
	frame()
	h.gpuSince = time.Now().Add(-2 * time.Minute)
	g.fail = true
	fail = true
	frame()
	if h.backoff != time.Second {
		t.Fatal("stable renderer did not reset retry delay", h.backoff)
	}
}
