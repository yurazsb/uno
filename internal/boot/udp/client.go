package udp

import (
	"context"
	"fmt"
	"github.com/yurazsb/uno/internal/boot"
	"github.com/yurazsb/uno/internal/boot/conn"
	"github.com/yurazsb/uno/internal/conf"
	"github.com/yurazsb/uno/internal/hook"
	"net"
	"sync"
)

type Client struct {
	address string
	ctx     context.Context
	cfg     *conf.Config
	hook    hook.ConnHook
	log     boot.Logger
	wg      *sync.WaitGroup
}

func NewClient(ctx context.Context, cfg conf.Config, hook hook.ConnHook, addr string) *Client {
	return &Client{
		address: addr,
		ctx:     ctx,
		cfg:     &cfg,
		hook:    hook,
		log:     cfg.Logger,
		wg:      &sync.WaitGroup{},
	}
}

func (c *Client) Dial() (boot.Conn, error) {
	network := c.cfg.Network
	rAddr, err := net.ResolveUDPAddr(network, c.address)
	if err != nil {
		return nil, fmt.Errorf("resolve addr failed: %w", err)
	}

	var lAddr *net.UDPAddr
	if c.cfg.LocalAddr != "" {
		lAddr, _ = net.ResolveUDPAddr(network, c.cfg.LocalAddr)
	}

	raw, err := net.DialUDP(network, lAddr, rAddr)
	if err != nil {
		return nil, fmt.Errorf("dial udp failed: %w", err)
	}

	c.log.Debug("Dial conn: " + raw.RemoteAddr().String())

	nc := conn.NewNETConn(c.ctx, raw, c.cfg, c.hook)
	nc.Start(c.wg)

	return nc, nil
}
