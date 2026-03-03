package bot

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot/models"
	log "github.com/sirupsen/logrus"
)

const overloadWarnInterval = 5 * time.Second

type updateHandler func(context.Context, models.Update)

type queuedUpdate struct {
	ctx    context.Context
	update models.Update
}

// updatePool ограничивает конкуррентность обработки апдейтов через очередь и фиксированное число воркеров.
type updatePool struct {
	updatesCh     chan queuedUpdate
	wg            sync.WaitGroup
	producersWg   sync.WaitGroup
	workerCount   int
	queueSize     int
	handle        updateHandler
	stopping      atomic.Bool
	warnThreshold float64
	lastWarnAtNs  atomic.Int64

	chatLocksMu sync.Mutex
	chatLocks   map[int64]*sync.Mutex
}

func newUpdatePool(workerCount, queueSize int, handle updateHandler) *updatePool {
	if workerCount <= 0 {
		workerCount = 1
	}
	if queueSize <= 0 {
		queueSize = workerCount
	}

	return &updatePool{
		updatesCh:     make(chan queuedUpdate, queueSize),
		workerCount:   workerCount,
		queueSize:     queueSize,
		handle:        handle,
		warnThreshold: 0.8,
		chatLocks:     make(map[int64]*sync.Mutex),
	}
}

func (p *updatePool) Start() {
	p.wg.Add(p.workerCount)
	for i := 0; i < p.workerCount; i++ {
		go func() {
			defer p.wg.Done()
			for item := range p.updatesCh {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("panic in update worker: %v", r)
						}
					}()

					chatID, hasChatID := updateChatID(item.update)
					if hasChatID {
						mu := p.lockFor(chatID)
						mu.Lock()
						defer mu.Unlock()
					}

					p.handle(item.ctx, item.update)
				}()
			}
		}()
	}
}

func (p *updatePool) Enqueue(ctx context.Context, update models.Update) {
	if p.stopping.Load() {
		return
	}

	p.producersWg.Add(1)
	defer p.producersWg.Done()

	if p.stopping.Load() {
		return
	}

	if p.queueSize > 0 {
		fillRatio := float64(len(p.updatesCh)+1) / float64(p.queueSize)
		if fillRatio >= p.warnThreshold && p.shouldLogOverloadWarn(time.Now()) {
			log.WithFields(log.Fields{
				"queue_len":  len(p.updatesCh),
				"queue_cap":  cap(p.updatesCh),
				"fill_ratio": fillRatio,
			}).Warn("update queue is close to full")
		}
	}

	select {
	case <-ctx.Done():
		return
	case p.updatesCh <- queuedUpdate{ctx: ctx, update: update}:
	}
}

func (p *updatePool) shouldLogOverloadWarn(now time.Time) bool {
	nowNs := now.UnixNano()
	last := p.lastWarnAtNs.Load()
	if last != 0 && nowNs-last < overloadWarnInterval.Nanoseconds() {
		return false
	}
	return p.lastWarnAtNs.CompareAndSwap(last, nowNs)
}

func (p *updatePool) lockFor(chatID int64) *sync.Mutex {
	if chatID == 0 {
		return nil
	}

	p.chatLocksMu.Lock()
	defer p.chatLocksMu.Unlock()

	if mu, ok := p.chatLocks[chatID]; ok {
		return mu
	}

	mu := &sync.Mutex{}
	p.chatLocks[chatID] = mu
	// TODO: добавить cleanup неиспользуемых chat lock'ов (TTL/LRU) при необходимости.
	return mu
}

func updateChatID(update models.Update) (int64, bool) {
	if update.Message != nil {
		if update.Message.Chat.ID != 0 {
			return update.Message.Chat.ID, true
		}
	}
	if update.EditedMessage != nil {
		if update.EditedMessage.Chat.ID != 0 {
			return update.EditedMessage.Chat.ID, true
		}
	}
	if update.ChannelPost != nil {
		if update.ChannelPost.Chat.ID != 0 {
			return update.ChannelPost.Chat.ID, true
		}
	}
	if update.EditedChannelPost != nil {
		if update.EditedChannelPost.Chat.ID != 0 {
			return update.EditedChannelPost.Chat.ID, true
		}
	}
	if update.BusinessMessage != nil {
		if update.BusinessMessage.Chat.ID != 0 {
			return update.BusinessMessage.Chat.ID, true
		}
	}
	if update.EditedBusinessMessage != nil {
		if update.EditedBusinessMessage.Chat.ID != 0 {
			return update.EditedBusinessMessage.Chat.ID, true
		}
	}
	if update.CallbackQuery != nil {
		if update.CallbackQuery.Message.Message != nil {
			if update.CallbackQuery.Message.Message.Chat.ID != 0 {
				return update.CallbackQuery.Message.Message.Chat.ID, true
			}
		}
		if update.CallbackQuery.Message.InaccessibleMessage != nil {
			if update.CallbackQuery.Message.InaccessibleMessage.Chat.ID != 0 {
				return update.CallbackQuery.Message.InaccessibleMessage.Chat.ID, true
			}
		}
	}

	return 0, false
}

func (p *updatePool) Stop() {
	if !p.stopping.CompareAndSwap(false, true) {
		return
	}
	p.producersWg.Wait()
	close(p.updatesCh)
	p.wg.Wait()
}
