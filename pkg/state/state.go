package state

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// /////////////////////////////////////////////////////////////////////////////
// Core types
// /////////////////////////////////////////////////////////////////////////////

type State int

func (s State) Clone() State { return s }

type Event int

func (e Event) Clone() Event { return e }

type Stage int

func (e Stage) Clone() Stage { return e }

const (
	StageGuards Stage = iota
	StageBefore
	StageApply
	StageAfter
	StageOnExit
	StageOnEnter
)

type Transition struct {
	From  State
	To    State
	Event Event
	Ctx   context.Context
	Time  time.Time
	Err   error
}

type HookFunc func(t *Transition) error

type HookSpec struct {
	Stage    Stage
	From     State
	To       State
	Priority int
	Timeout  time.Duration
	Fn       HookFunc
}

///////////////////////////////////////////////////////////////////////////////
// Transition table
///////////////////////////////////////////////////////////////////////////////

type transitionTable struct {
	mu    sync.RWMutex
	state map[State]State
	event map[State]map[Event]State
}

func newTransitions() *transitionTable {
	return &transitionTable{
		state: make(map[State]State),
		event: map[State]map[Event]State{},
	}
}

func (t *transitionTable) Add(from State, to State, evs ...Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(evs) > 0 {
		if t.event[from] == nil {
			t.event[from] = map[Event]State{}
		}
		for _, ev := range evs {
			t.event[from][ev] = to
		}
	} else {
		t.state[from] = to
	}
}

func (t *transitionTable) Enable(from State, to State) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	toState, ok := t.state[from]
	return ok && to == toState
}

func (t *transitionTable) Next(from State, ev Event) (State, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	evs := t.event[from]
	if evs == nil {
		return 0, false
	}
	s, ok := evs[ev]
	return s, ok
}

///////////////////////////////////////////////////////////////////////////////
// Hooks
///////////////////////////////////////////////////////////////////////////////

type hooks struct {
	mu sync.RWMutex
	m  map[Stage][]HookSpec
}

func newHooks() *hooks {
	return &hooks{m: make(map[Stage][]HookSpec)}
}

func (r *hooks) Add(h HookSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[h.Stage] = append(r.m[h.Stage], h)
	// keep sorted by Priority ascending
	sort.SliceStable(r.m[h.Stage], func(i, j int) bool {
		return r.m[h.Stage][i].Priority < r.m[h.Stage][j].Priority
	})
}

func (r *hooks) Run(stage Stage, t *Transition) error {
	r.mu.RLock()
	hs := append([]HookSpec(nil), r.m[stage]...)
	r.mu.RUnlock()

	for _, h := range hs {
		if !matchHook(&h, t) {
			continue
		}
		// prepare context with timeout if needed
		ctx := t.Ctx
		var cancel context.CancelFunc
		if h.Timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, h.Timeout)
		}
		err := safeInvoke(ctx, h.Fn, t)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func matchHook(h *HookSpec, t *Transition) bool {
	if h.From != t.From {
		return false
	}
	if h.To != t.To {
		return false
	}
	return true
}

func safeInvoke(ctx context.Context, fn HookFunc, t *Transition) (err error) {
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic in hook: %v", r)
			}
		}()
		done <- fn(t)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-done:
		return e
	}
}

///////////////////////////////////////////////////////////////////////////////
// State Machine
///////////////////////////////////////////////////////////////////////////////

type Snapshot struct {
	State State
	Epoch uint64
	Since time.Time
}

const (
	changeCmd = iota
	eventCmd
)

type cmd struct {
	cmdType int
	value   any
	ctx     context.Context
	resp    chan error
}

type Machine struct {
	name string

	cur   State
	epoch uint64
	snap  atomic.Value

	cmdCh chan cmd

	transitions *transitionTable
	hooks       *hooks

	startOnce sync.Once

	stopOnce sync.Once
	stopped  chan struct{}
}

func NewMachine(name string, initial State) *Machine {
	s := &Machine{
		name:        name,
		cur:         initial,
		epoch:       0,
		cmdCh:       make(chan cmd, 64),
		transitions: newTransitions(),
		hooks:       newHooks(),
		stopped:     make(chan struct{}),
	}
	s.snap.Store(Snapshot{State: initial, Epoch: 0, Since: time.Now()})
	return s
}

func (s *Machine) Name() string { return s.name }
func (s *Machine) Snapshot() Snapshot {
	return s.snap.Load().(Snapshot)
}

func (s *Machine) AddTransition(from State, to State, evs ...Event) {
	s.transitions.Add(from, to, evs...)
}

func (s *Machine) RegHook(hook HookSpec) {
	s.hooks.Add(hook)
}

func (s *Machine) Change(ctx context.Context, state State) error {
	c := cmd{cmdType: changeCmd, value: state, ctx: ctx, resp: make(chan error, 1)}
	return s.resp(c)
}

func (s *Machine) Event(ctx context.Context, event Event) error {
	c := cmd{cmdType: eventCmd, value: event, ctx: ctx, resp: make(chan error, 1)}
	return s.resp(c)
}

func (s *Machine) Run() {
	s.startOnce.Do(func() {
		go s.loop()
	})
}

func (s *Machine) Stop() {
	s.stopOnce.Do(func() {
		close(s.cmdCh)
		<-s.stopped
	})
}

func (s *Machine) resp(c cmd) error {
	select {
	case s.cmdCh <- c:
	case <-c.ctx.Done():
		return c.ctx.Err()
	}

	select {
	case err := <-c.resp:
		return err
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (s *Machine) loop() {
	defer close(s.stopped)
	for c := range s.cmdCh {
		_ = s.handleCmd(c)
	}
}

func (s *Machine) handleCmd(c cmd) error {
	from := s.cur
	var ok bool
	var to State
	var ev Event

	if c.cmdType == changeCmd {
		to = c.value.(State)
		ok = s.transitions.Enable(from, to)
	} else {
		ev = c.value.(Event)
		to, ok = s.transitions.Next(from, ev)
	}

	if !ok {
		select {
		case c.resp <- fmt.Errorf("undefined transition"):
		default:
		}
		return nil
	}

	t := &Transition{
		From:  from,
		To:    to,
		Event: ev,
		Ctx:   c.ctx,
		Time:  time.Now(),
	}

	// Guards
	if err := s.hooks.Run(StageGuards, t); err != nil {
		t.Err = err
		select {
		case c.resp <- err:
		default:
		}
		return err
	}

	// Before
	if err := s.hooks.Run(StageBefore, t); err != nil {
		t.Err = err
		select {
		case c.resp <- err:
		default:
		}
		return err
	}

	s.cur = to
	s.epoch++
	s.snap.Store(Snapshot{State: s.cur, Epoch: s.epoch, Since: time.Now()})
	_ = s.hooks.Run(StageApply, t)

	if err := s.hooks.Run(StageAfter, t); err != nil {
		t.Err = err
		select {
		case c.resp <- err:
		default:
		}
		return err
	}

	_ = s.hooks.Run(StageOnExit, t)
	_ = s.hooks.Run(StageOnEnter, t)

	select {
	case c.resp <- nil:
	default:
	}
	return nil
}
