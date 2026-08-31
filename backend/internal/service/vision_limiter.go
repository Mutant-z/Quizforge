package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type adaptiveVisionLimiter struct {
	mu                 sync.Mutex
	notify             chan struct{}
	active             int
	limit              int
	max                int
	successes          int
	durations          []time.Duration
	timeouts           []time.Time
	circuitUntil       time.Time
	probePending       bool
	circuitDuration    time.Duration
	noProgressDuration time.Duration
	lastProgress       time.Time
	degradedReason     string
}

type visionPermit struct {
	limiter *adaptiveVisionLimiter
	queue   time.Duration
	probe   bool
}

func newAdaptiveVisionLimiter(initial, max int, circuitDuration, noProgressDuration time.Duration) *adaptiveVisionLimiter {
	if initial < 1 {
		initial = 1
	}
	if max < initial {
		max = initial
	}
	return &adaptiveVisionLimiter{notify: make(chan struct{}), limit: initial, max: max, circuitDuration: circuitDuration, noProgressDuration: noProgressDuration, lastProgress: time.Now()}
}

func (l *adaptiveVisionLimiter) acquire(ctx context.Context) (*visionPermit, error) {
	queued := time.Now()
	for {
		l.mu.Lock()
		now := time.Now()
		if l.noProgressDuration > 0 && l.active > 0 && now.Sub(l.lastProgress) >= l.noProgressDuration {
			l.limit = 1
			l.degradedReason = "长时间没有完成单元，已暂停创建并发请求并降为单路恢复"
		}
		if !l.circuitUntil.IsZero() && now.Before(l.circuitUntil) {
			wait := time.Until(l.circuitUntil)
			notify := l.notify
			l.mu.Unlock()
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-notify:
				timer.Stop()
			case <-timer.C:
			}
			continue
		}
		if !l.circuitUntil.IsZero() && !now.Before(l.circuitUntil) {
			l.circuitUntil = time.Time{}
			l.probePending = true
			l.limit = 1
		}
		if l.active < l.limit {
			l.active++
			probe := l.probePending
			l.probePending = false
			l.mu.Unlock()
			return &visionPermit{limiter: l, queue: time.Since(queued), probe: probe}, nil
		}
		notify := l.notify
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-notify:
		}
	}
}

func (p *visionPermit) release(err error, duration time.Duration) {
	l := p.limiter
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active > 0 {
		l.active--
	}
	now := time.Now()
	if err == nil {
		l.lastProgress = now
		l.successes++
		l.durations = append(l.durations, duration)
		if len(l.durations) > 20 {
			l.durations = l.durations[len(l.durations)-20:]
		}
		if l.successes >= 10 && l.limit < l.max && percentile75(l.durations) < 60*time.Second {
			l.limit++
			l.successes = 0
			l.degradedReason = ""
		}
	} else if isVisionThrottleError(err) {
		l.limit = 1
		l.successes = 0
		l.degradedReason = "模型超时或限流，已自动降低并发"
		l.timeouts = append(l.timeouts, now)
		cutoff := now.Add(-5 * time.Minute)
		kept := l.timeouts[:0]
		for _, value := range l.timeouts {
			if value.After(cutoff) {
				kept = append(kept, value)
			}
		}
		l.timeouts = kept
		if len(l.timeouts) >= 3 {
			l.circuitUntil = now.Add(l.circuitDuration)
			l.degradedReason = "视觉模型连续超时，熔断后将以单图探测恢复"
		}
	}
	close(l.notify)
	l.notify = make(chan struct{})
}

func (l *adaptiveVisionLimiter) snapshot() (int, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit, l.degradedReason
}

func percentile75(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]time.Duration(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	index := (len(copyValues)*3+3)/4 - 1
	if index < 0 {
		index = 0
	}
	return copyValues[index]
}

func isVisionThrottleError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "deadline exceeded") || strings.Contains(message, "timeout") || strings.Contains(message, "429") || strings.Contains(message, "too many requests")
}

var visionLimiterRegistry = struct {
	sync.Mutex
	items map[string]*adaptiveVisionLimiter
}{items: map[string]*adaptiveVisionLimiter{}}

func visionLimiterFor(key string, initial, max int, circuit, noProgress time.Duration) *adaptiveVisionLimiter {
	visionLimiterRegistry.Lock()
	defer visionLimiterRegistry.Unlock()
	if existing := visionLimiterRegistry.items[key]; existing != nil {
		return existing
	}
	created := newAdaptiveVisionLimiter(initial, max, circuit, noProgress)
	visionLimiterRegistry.items[key] = created
	return created
}
