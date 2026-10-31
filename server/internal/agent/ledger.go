package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"safeops/internal/core"
	"safeops/internal/store"
)

type ledgerState struct {
	Version int                `json:"version"`
	Legacy  map[string]bool    `json:"legacy"`
	Uses    map[string]bool    `json:"uses"`
	Recent  map[string][]int64 `json:"recent"`
}
type Ledger struct {
	mu       sync.Mutex
	path     string
	state    ledgerState
	lock     *os.File
	poisoned error
}

func OpenLedger(path string) (*Ledger, error) {
	if path == "" {
		return nil, errors.New("persistent replay ledger required")
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("agent ledger already in use")
	}
	l := &Ledger{path: path, lock: f, state: ledgerState{Version: 2, Legacy: map[string]bool{}, Uses: map[string]bool{}, Recent: map[string][]int64{}}}
	b, e := os.ReadFile(path)
	if e == nil {
		var old map[string]bool
		if json.Unmarshal(b, &old) == nil && old != nil {
			l.state.Legacy = old
		} else {
			var state ledgerState
			e = json.Unmarshal(b, &state)
			l.state = state
			if e == nil && (l.state.Version != 2 || l.state.Legacy == nil || l.state.Uses == nil || l.state.Recent == nil) {
				e = errors.New("invalid replay ledger")
			}
		}
	}
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		f.Close()
		return nil, e
	}
	return l, nil
}
func (l *Ledger) Close() error { return l.lock.Close() }
func (l *Ledger) save() error {
	if e := store.AtomicWrite(l.path, core.JSON(l.state)); e != nil {
		l.poisoned = e
		return e
	}
	return nil
}

// Consume retains legacy whole-request semantics for existing deployments.
func (l *Ledger) Consume(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.poisoned != nil {
		return l.poisoned
	}
	if l.state.Legacy[id] {
		return errors.New("request already consumed")
	}
	l.state.Legacy[id] = true
	return l.save()
}

// ConsumeGrant commits replay and local rate accounting before starting a
// process. A crash consumes the attempt; neither Broker nor restart retries it.
func (l *Ledger) ConsumeGrant(i core.Intent, lease core.Lease, rb core.Runbook, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.poisoned != nil {
		return l.poisoned
	}
	use := lease.Use
	if i.Version == 1 && lease.Version == 1 && use == 0 {
		use = 1
	}
	if rb.Review.Validate(rb.Quorum) != nil || use < 1 || use > core.Uses(i) || core.Uses(i) > rb.Review.Effective().MaxUses {
		return errors.New("invalid use budget")
	}
	key := core.Encode([]any{i.ID, use})
	if l.state.Legacy[i.ID] || l.state.Uses[key] {
		return errors.New("request use already consumed")
	}
	rateKey := core.Encode([]string{i.Target, i.Action, i.Requester})
	recent := []int64{}
	for _, at := range l.state.Recent[rateKey] {
		if at > now.Unix()-60 {
			recent = append(recent, at)
		}
	}
	if len(recent) >= rb.Review.Effective().RateLimit {
		return errors.New("local execution rate exceeded")
	}
	l.state.Uses[key] = true
	l.state.Recent[rateKey] = append(recent, now.Unix())
	return l.save()
}
