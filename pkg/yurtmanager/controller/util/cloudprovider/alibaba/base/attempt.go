package base

import (
	"io"
	"net"
	"net/url"
	"time"
)

// AttemptStrategy is reused from the goamz package

// AttemptStrategy represents a strategy for waiting for an action
// to complete successfully. This is an internal type used by the
// implementation of other packages.
type AttemptStrategy struct {
	Total time.Duration // total duration of attempt.
	Delay time.Duration // interval between each try in the burst.
	Min   int           // minimum number of retries; overrides Total
}

type Attempt struct {
	strategy AttemptStrategy
	last     time.Time
	end      time.Time
	force    bool
	count    int
}

// Start begins a new sequence of attempts for the given strategy.
func (s AttemptStrategy) Start() *Attempt {
	now := time.Now()
	return &Attempt{
		strategy: s,
		last:     now,
		end:      now.Add(s.Total),
		force:    true,
	}
}

// Next waits until it is time to perform the next attempt or returns
// false if it is time to stop trying.
func (a *Attempt) Next() bool {
	now := time.Now()
	sleep := a.nextSleep(now)
	if !a.force && !now.Add(sleep).Before(a.end) && a.strategy.Min <= a.count {
		return false
	}
	a.force = false
	if sleep > 0 && a.count > 0 {
		time.Sleep(sleep)
		now = time.Now()
	}
	a.count++
	a.last = now
	return true
}

func (a *Attempt) nextSleep(now time.Time) time.Duration {
	sleep := a.strategy.Delay - now.Sub(a.last)
	if sleep < 0 {
		return 0
	}
	return sleep
}

// HasNext returns whether another attempt will be made if the current
// one fails. If it returns true, the following call to Next is
// guaranteed to return true.
func (a *Attempt) HasNext() bool {
	if a.force || a.strategy.Min > a.count {
		return true
	}
	now := time.Now()
	if now.Add(a.nextSleep(now)).Before(a.end) {
		a.force = true
		return true
	}
	return false
}

type TimeoutError interface {
	error
	Timeout() bool // Is the error a timeout?
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	_, ok := err.(TimeoutError)
	if ok {
		return true
	}

	switch err {
	case io.ErrUnexpectedEOF, io.EOF:
		return true
	}
	switch e := err.(type) {
	case *net.DNSError:
		return true
	case *net.OpError:
		switch e.Op {
		case "read", "write":
			return true
		}
	case *url.Error:
		// url.Error can be returned either by net/url if a URL cannot be
		// parsed, or by net/http if the response is closed before the headers
		// are received or parsed correctly. In that later case, e.Op is set to
		// the HTTP method name with the first letter uppercased. We don't want
		// to retry on POST operations, since those are not idempotent, all the
		// other ones should be safe to retry.
		switch e.Op {
		case "Get", "Put", "Delete", "Head":
			return shouldRetry(e.Err)
		default:
			return false
		}
	}
	return false
}
