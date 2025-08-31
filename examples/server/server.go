package main

import (
	"context"
	"fmt"
	"github.com/yurazsb/uno"
	"github.com/yurazsb/uno/internal/boot"
	"log"
)

type EchoServer struct {
	uno.ServerEvent
}

func (e *EchoServer) OnStart(s boot.Server) {
	log.Printf("OnStart %s", s.Addr())
}
func (e *EchoServer) OnStop(s boot.Server) {
	log.Printf("OnStop %s", s.Addr())
}
func (e *EchoServer) OnConnect(c boot.Conn) {
	log.Printf("OnConnect %s", c.RemoteAddr().String())
}
func (e *EchoServer) OnClose(c boot.Conn) {
	log.Printf("OnClose %s", c.RemoteAddr().String())
}
func (e *EchoServer) OnError(c boot.Conn, err error) {
	log.Printf("OnError %s %v", c.RemoteAddr().String(), err)
}
func (e *EchoServer) OnTick(c boot.Conn) {
	log.Printf("OnTick %s", c.RemoteAddr().String())
}
func (e *EchoServer) OnIdle(c boot.Conn) {
	log.Printf("OnIdle %s", c.RemoteAddr().String())
}
func (e *EchoServer) OnSend(c boot.Conn, msg any) {
	log.Printf("OnSend %s %v", c.RemoteAddr().String(), msg)
}
func (e *EchoServer) OnWrite(c boot.Conn, buf []byte, err error) {
	log.Printf("OnWrite %s %v %v", c.RemoteAddr().String(), buf, err)
}
func (e *EchoServer) OnRead(c boot.Conn, buf []byte, err error) {
	log.Printf("OnRead %s %v %v", c.RemoteAddr().String(), buf, err)
}
func (e *EchoServer) OnMessage(c boot.Conn, msg any) {
	log.Printf("OnMessage %s %v %s", c.RemoteAddr().String(), msg, string(msg.([]byte)))
	done := c.Send("hello client!")
	err := <-done
	if err != nil {
		fmt.Printf("Send err %s %v", c.RemoteAddr().String(), err)
	}
	fmt.Printf("Send success %s\n", c.RemoteAddr().String())
}

func main() {
	network := "tcp"
	err := uno.Serve(context.Background(), &EchoServer{}, ":9090", uno.WithNetwork(network))
	if err != nil {
		fmt.Println(err)
	}
}
