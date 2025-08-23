package handler

import (
	"context"
	"github.com/yurazsb/uno/internal/boot"
	"github.com/yurazsb/uno/pkg/attrs"
	"sync/atomic"
)

type Handler func(ctx Context, next func())

type Context interface {
	Context() context.Context
	Cancel()
	Done() <-chan struct{}
	Err() error
	Attrs() boot.Attrs
	Conn() boot.Conn
	Payload() any
	SetPayload(payload any)
}

type Chain struct {
	handlers []Handler
}

func NewChain(h ...Handler) *Chain {
	return &Chain{
		handlers: append([]Handler{}, h...),
	}
}

func (c *Chain) Use(h ...Handler) *Chain {
	c.handlers = append(c.handlers, h...)
	return c
}

// Handler 执行
func (c *Chain) Handler(ctx Context) {
	if len(c.handlers) == 0 || ctx == nil {
		return
	}

	var dispatch func(index int)
	dispatch = func(index int) {
		if index >= len(c.handlers) {
			return
		}

		h := c.handlers[index]

		h(ctx, func() {
			dispatch(index + 1)
		})
	}

	dispatch(0)
}

type hContext struct {
	conn    boot.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	payload atomic.Value
	attrs   boot.Attrs
}

func NewContext(conn boot.Conn, payload any) Context {
	ctx, cancel := context.WithCancel(conn.Context())
	c := &hContext{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
		attrs:  attrs.New[any, any](true),
	}
	c.payload.Store(payload)
	return c
}

func (c *hContext) Context() context.Context { return c.ctx }
func (c *hContext) Cancel()                  { c.cancel() }
func (c *hContext) Done() <-chan struct{}    { return c.ctx.Done() }
func (c *hContext) Err() error               { return c.ctx.Err() }
func (c *hContext) Attrs() boot.Attrs        { return c.attrs }
func (c *hContext) Conn() boot.Conn          { return c.conn }
func (c *hContext) Payload() any             { return c.payload.Load() }
func (c *hContext) SetPayload(p any)         { c.payload.Store(p) }
