package requestevents

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type WriterConfig struct {
	ChannelSize    int
	BatchSize      int
	FlushInterval  time.Duration
	OnOverflowDrop bool
}

type WriterStatus struct {
	Queued      int64 `json:"queued"`
	Dropped     int64 `json:"dropped"`
	LastFlushAt int64 `json:"last_flush_at"`
	Inserted    int64 `json:"inserted"`
	Skipped     int64 `json:"skipped"`
	DeadLetters int64 `json:"dead_letters"`
}

type Writer struct {
	store  *Store
	cfg    WriterConfig
	ch     chan []byte
	cancel context.CancelFunc
	wg     sync.WaitGroup

	queued      atomic.Int64
	dropped     atomic.Int64
	lastFlushAt atomic.Int64
	inserted    atomic.Int64
	skipped     atomic.Int64
	deadLetters atomic.Int64
}

func NewWriter(store *Store, cfg WriterConfig) *Writer {
	if cfg.ChannelSize <= 0 {
		cfg.ChannelSize = 4096
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 500 * time.Millisecond
	}
	return &Writer{
		store: store,
		cfg:   cfg,
		ch:    make(chan []byte, cfg.ChannelSize),
	}
}

func (w *Writer) Start(parent context.Context) {
	if w == nil || w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go w.run(ctx)
}

func (w *Writer) Stop() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
	w.cancel = nil
}

func (w *Writer) Emit(raw []byte) {
	if w == nil || len(raw) == 0 {
		return
	}
	payload := append([]byte(nil), raw...)
	select {
	case w.ch <- payload:
		w.queued.Add(1)
	default:
		if w.cfg.OnOverflowDrop {
			select {
			case dropped := <-w.ch:
				_ = dropped
				w.dropped.Add(1)
			default:
				w.dropped.Add(1)
				return
			}
			select {
			case w.ch <- payload:
				w.queued.Add(1)
			default:
				w.dropped.Add(1)
			}
			return
		}
		w.dropped.Add(1)
	}
}

func (w *Writer) Status() WriterStatus {
	return WriterStatus{
		Queued:      w.queued.Load(),
		Dropped:     w.dropped.Load(),
		LastFlushAt: w.lastFlushAt.Load(),
		Inserted:    w.inserted.Load(),
		Skipped:     w.skipped.Load(),
		DeadLetters: w.deadLetters.Load(),
	}
}

func (w *Writer) run(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]Event, 0, w.cfg.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		result, err := w.store.InsertEvents(ctx, batch)
		if err != nil {
			for _, event := range batch {
				_ = w.store.AddDeadLetter(ctx, event.RawJSON, err)
				w.deadLetters.Add(1)
			}
		} else {
			w.inserted.Add(int64(result.Inserted))
			w.skipped.Add(int64(result.Skipped))
		}
		batch = batch[:0]
		w.lastFlushAt.Store(time.Now().UnixMilli())
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case raw := <-w.ch:
			event, err := NormalizeRaw(raw)
			if err != nil {
				_ = w.store.AddDeadLetter(ctx, string(raw), err)
				w.deadLetters.Add(1)
				continue
			}
			batch = append(batch, event)
			if len(batch) >= w.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
