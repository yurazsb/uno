package udp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"uno/internal/boot"
	"uno/internal/boot/conn"
	"uno/internal/conf"
	"uno/internal/hook"
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
