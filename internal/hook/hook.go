package hook

import (
	"uno/internal/boot"
)

type ServerHook interface {
	OnStart(s boot.Server)
	OnStop(s boot.Server)
	ConnHook
}

type ConnHook interface {
	OnConnect(c boot.Conn)
	OnClose(c boot.Conn)
	OnError(c boot.Conn, err error)
	OnTick(c boot.Conn)
	OnIdle(c boot.Conn)
	OnSend(c boot.Conn, msg any)
	OnWrite(c boot.Conn, buf []byte, err error)
	OnRead(c boot.Conn, buf []byte, err error)
	OnMessage(c boot.Conn, msg any)
}

type ServerEvent struct {
	ConnEvent
}

func (e *ServerEvent) OnStart(s boot.Server) {}

func (e *ServerEvent) OnStop(s boot.Server) {}

type ConnEvent struct{}

func (e *ConnEvent) OnConnect(c boot.Conn)                      {}
func (e *ConnEvent) OnClose(c boot.Conn)                        {}
func (e *ConnEvent) OnError(c boot.Conn, err error)             {}
func (e *ConnEvent) OnTick(c boot.Conn)                         {}
func (e *ConnEvent) OnIdle(c boot.Conn)                         {}
func (e *ConnEvent) OnSend(c boot.Conn, msg any)                {}
func (e *ConnEvent) OnWrite(c boot.Conn, buf []byte, err error) {}
func (e *ConnEvent) OnRead(c boot.Conn, buf []byte, err error)  {}
func (e *ConnEvent) OnMessage(c boot.Conn, msg any)             {}
