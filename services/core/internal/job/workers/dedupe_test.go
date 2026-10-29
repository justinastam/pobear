package workers_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/Decodo/pobear/services/core/internal/job/workers"
)

func TestDedupeClaimIsSafeForConcurrentCallers(t *testing.T) {
	d := workers.NewDedupe()

	var wg sync.WaitGroup
	var mu sync.Mutex
	claimed := 0
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(2)
		claim := func() {
			defer wg.Done()
			if d.Claim(fmt.Sprintf("https://a.test/%d", i)) {
				mu.Lock()
				claimed++
				mu.Unlock()
			}
		}
		go claim()
		go claim()
	}
	wg.Wait()

	if claimed != 50 {
		t.Fatalf("claimed = %d, want exactly 50 (one winner per URL, race detector should also be clean)", claimed)
	}
}
