package boot

import (
	"context"
	"net"
	"uno/pkg/attrs"
)

type Server interface {
	Addr() net.Addr
	Context() context.Context
	IsRunning() bool
	Stop()
}

type Client interface {
	Dial() (Conn, error)
}

type Conn interface {
	ID() string
	Context() context.Context
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Attrs() Attrs
	IsActive() bool
	Send(msg any) error
	Close()
}

type Attrs = attrs.Attrs[any, any]

type Pool interface {
	Submit(task func()) bool
}

type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}
