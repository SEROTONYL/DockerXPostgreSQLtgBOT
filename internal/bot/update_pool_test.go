package bot

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func TestUpdatePool_ProcessesAllUpdatesAndRespectsWorkerLimit(t *testing.T) {
	const (
		workerCount = 2
		queueSize   = 3
		total       = 5
	)

	var processed atomic.Int32
	var inFlight atomic.Int32
	var maxSeen atomic.Int32

	pool := newUpdatePool(workerCount, queueSize, func(_ context.Context, _ models.Update) {
		current := inFlight.Add(1)
		for {
			prev := maxSeen.Load()
			if current <= prev || maxSeen.CompareAndSwap(prev, current) {
				break
			}
		}

		time.Sleep(20 * time.Millisecond)
		processed.Add(1)
		inFlight.Add(-1)
	})

	pool.Start()

	for i := 0; i < total; i++ {
		pool.Enqueue(context.Background(), models.Update{ID: int64(i + 1)})
	}

	pool.Stop()

	if got := processed.Load(); got != total {
		t.Fatalf("processed = %d, want %d", got, total)
	}

	if got := maxSeen.Load(); got > workerCount {
		t.Fatalf("max concurrent handlers = %d, want <= %d", got, workerCount)
	}
}

func TestUpdatePool_RecoversFromPanic(t *testing.T) {
	var processed atomic.Int32
	pool := newUpdatePool(1, 2, func(_ context.Context, update models.Update) {
		if update.ID == 1 {
			panic("boom")
		}
		processed.Add(1)
	})

	pool.Start()
	pool.Enqueue(context.Background(), models.Update{ID: 1})
	pool.Enqueue(context.Background(), models.Update{ID: 2})
	pool.Stop()

	if got := processed.Load(); got != 1 {
		t.Fatalf("processed after panic = %d, want 1", got)
	}
}

func TestUpdatePool_SequentialPerChatAndParallelAcrossChats(t *testing.T) {
	pool := newUpdatePool(2, 4, nil)

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	otherChatStarted := make(chan struct{})

	var inFlight atomic.Int32
	var maxSeen atomic.Int32
	var orderMu sync.Mutex
	order := make([]int64, 0, 2)

	pool.handle = func(_ context.Context, update models.Update) {
		current := inFlight.Add(1)
		for {
			prev := maxSeen.Load()
			if current <= prev || maxSeen.CompareAndSwap(prev, current) {
				break
			}
		}
		defer inFlight.Add(-1)

		switch update.ID {
		case 1:
			close(firstStarted)
			<-releaseFirst
			orderMu.Lock()
			order = append(order, 1)
			orderMu.Unlock()
		case 2:
			orderMu.Lock()
			order = append(order, 2)
			orderMu.Unlock()
		case 3:
			close(otherChatStarted)
			time.Sleep(20 * time.Millisecond)
		}
	}

	pool.Start()
	pool.Enqueue(context.Background(), models.Update{ID: 1, Message: &models.Message{Chat: models.Chat{ID: 100}}})

	select {
	case <-firstStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("first update did not start")
	}

	pool.Enqueue(context.Background(), models.Update{ID: 3, Message: &models.Message{Chat: models.Chat{ID: 200}}})
	pool.Enqueue(context.Background(), models.Update{ID: 2, Message: &models.Message{Chat: models.Chat{ID: 100}}})

	select {
	case <-otherChatStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("update from other chat did not run in parallel")
	}

	close(releaseFirst)
	pool.Stop()

	orderMu.Lock()
	defer orderMu.Unlock()
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("order for same chat = %v, want [1 2]", order)
	}

	if got := maxSeen.Load(); got <= 1 {
		t.Fatalf("max concurrency = %d, want > 1 for different chats", got)
	}
}
