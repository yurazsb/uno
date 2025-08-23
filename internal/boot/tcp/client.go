package tcp

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

func (c Client) Dial() (boot.Conn, error) {
	network := c.cfg.Network

	// 解析远程地址
	rAddr, err := net.ResolveTCPAddr(network, c.address)
	if err != nil {
		return nil, fmt.Errorf("resolve udp addr failed: %w", err)
	}

	// 解析本地地址（可选）
	var lAddr *net.TCPAddr
	if c.cfg.LocalAddr != "" {
		lAddr, err = net.ResolveTCPAddr(network, c.cfg.LocalAddr)
		if err != nil {
			return nil, fmt.Errorf("resolve local udp addr failed: %w", err)
		}
	}

	raw, err := net.DialTCP(network, lAddr, rAddr)
	if err != nil {
		return nil, fmt.Errorf("dial tcp failed: %w", err)
	}

	c.log.Debug("Dial conn: " + raw.RemoteAddr().String())

	nc := conn.NewNETConn(c.ctx, raw, c.cfg, c.hook)
	nc.Start(c.wg)

	return nc, nil
}
