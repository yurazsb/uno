package main

import (
	"context"
	"fmt"
	"github.com/yurazsb/uno"
	"github.com/yurazsb/uno/internal/boot"
	"log"
	"time"
)

type EchoClient struct {
	uno.ConnEvent
}

func (e *EchoClient) OnConnect(c boot.Conn) {
	log.Printf("OnConnect %s", c.RemoteAddr().String())
}
func (e *EchoClient) OnClose(c boot.Conn) {
	log.Printf("OnClose %s", c.RemoteAddr().String())
}
func (e *EchoClient) OnError(c boot.Conn, err error) {
	log.Printf("OnError %s %v", c.RemoteAddr().String(), err)
}
func (e *EchoClient) OnTick(c boot.Conn) {
	log.Printf("OnTick %s", c.RemoteAddr().String())
}
func (e *EchoClient) OnIdle(c boot.Conn) {
	log.Printf("OnIdle %s", c.RemoteAddr().String())
}
func (e *EchoClient) OnSend(c boot.Conn, msg any) {
	log.Printf("OnSend %s %v", c.RemoteAddr().String(), msg)
}
func (e *EchoClient) OnWrite(c boot.Conn, buf []byte, err error) {
	log.Printf("OnWrite %s %v %v", c.RemoteAddr().String(), buf, err)
}
func (e *EchoClient) OnRead(c boot.Conn, buf []byte, err error) {
	log.Printf("OnRead %s %v %v", c.RemoteAddr().String(), buf, err)
}
func (e *EchoClient) OnMessage(c boot.Conn, msg any) {
	log.Printf("OnMessage %s %v %s", c.RemoteAddr().String(), msg, string(msg.([]byte)))
}

func main() {
	network := "tcp"
	conn, err := uno.Dial(context.Background(), &EchoClient{}, "127.0.0.1:9090", uno.WithNetwork(network))
	if err != nil {
		fmt.Println(err)
	}

	tick := time.NewTicker(time.Second * 2)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			done := conn.Send("hello server!")
			err := <-done
			if err != nil {
				fmt.Printf("Send err %v", err)
			}
			fmt.Printf("Send success\n")
		}
	}
}
