package udp

import (
	"context"
	"errors"
	"github.com/yurazsb/uno/internal/boot"
	"github.com/yurazsb/uno/internal/boot/conn"
	"github.com/yurazsb/uno/internal/conf"
	"github.com/yurazsb/uno/internal/hook"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Server 是 UDP 的“伪连接”服务端。
// 单个 UDP socket 上，按 remote(IP:port) 多路复用出多个逻辑连接（SConn）
// 每个逻辑连接都包装为 *conn.Conn，具备完整的编解码、拆帧、Hook、队列化写等能力。
type Server struct {
	us *conn.UDPSession

	address string
	addr    net.Addr
	uc      *net.UDPConn

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc

	// 依赖
	cfg  *conf.Config
	pool boot.Pool
	log  boot.Logger
	hook hook.ServerHook

	running atomic.Bool

	started atomic.Bool

	stopOnce sync.Once
	stopped  chan struct{}

	wg *sync.WaitGroup

	// 可选：空闲连接清理
	reapStop chan struct{}
}

// NewServer 创建 UDP 服务器实例（未监听）。
func NewServer(ctx context.Context, cfg conf.Config, hook hook.ServerHook, addr string) *Server {
	c, cancel := context.WithCancel(ctx)
	return &Server{
		address:  addr,
		ctx:      c,
		cancel:   cancel,
		cfg:      &cfg,
		pool:     cfg.Pool,
		log:      cfg.Logger,
		hook:     hook,
		wg:       &sync.WaitGroup{},
		stopped:  make(chan struct{}),
		reapStop: make(chan struct{}),
	}
}

func (s *Server) Addr() net.Addr           { return s.addr }
func (s *Server) Context() context.Context { return s.ctx }
func (s *Server) IsRunning() bool          { return s.running.Load() }

func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		<-s.stopped
	})
}

func (s *Server) Listen() error {
	if s.started.Load() {
		return errors.New("already started")
	}

	s.started.Store(true)
	network := s.cfg.Network // "udp", "udp4", "udp6"
	udpAddr, err := net.ResolveUDPAddr(network, s.address)
	if err != nil {
		return err
	}

	s.uc, err = net.ListenUDP(network, udpAddr)
	if err != nil {
		return err
	}

	s.us = conn.NewUDPSession(s.uc, s.cfg, s.hook)
	s.addr = s.uc.LocalAddr()
	s.log.Debug("listening on %s://%s", s.cfg.Network, s.addr.String())

	task := func() { s.hook.OnStart(s) }
	if !s.pool.Submit(task) {
		s.log.Error("fail to submit task: %v", task)
	}

	// 空闲连接清理（可选）
	idleTimeout := s.cfg.IdleTimeout
	if idleTimeout > 0 {
		idleTimeout = 5 * time.Minute
	}
	go s.reaper(idleTimeout)

	// 主循环
	return s.serve()
}

func (s *Server) serve() error {
	defer s.clear()

	// 主读缓冲可复用，但每次要 Clone 给下游，避免数据竞争
	buf := make([]byte, s.cfg.ReadBufferSize)

	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
		}

		// 可选读超时（避免永久阻塞，便于响应 Stop）此处实际用于接收连接
		_ = s.uc.SetReadDeadline(time.Now().Add(conn.AcceptTimeout))
		nr, raddr, err := s.uc.ReadFromUDP(buf)

		// 先处理有效数据（即便 err != nil，也要先处理 nc>0 的数据）
		if nr > 0 {
			chunk := make([]byte, nr)
			copy(chunk, buf[:nr])
			s.us.Delivery(s.ctx, s.wg, raddr, chunk)
		}

		if err != nil {
			// 被 Stop() 关闭或 Context 取消
			if s.ctx.Err() != nil {
				return nil
			}

			// 其他错误：记录并继续
			continue
		}
	}
}

// 清理空闲连接：定时扫描 connMap，超过 idle 超时的连接关闭并移除
func (s *Server) reaper(idle time.Duration) {
	s.wg.Add(1)
	defer s.wg.Done()

	tk := time.NewTicker(idle / 2)
	defer tk.Stop()

	for {
		select {
		case <-s.reapStop:
			return
		case <-s.ctx.Done():
			return
		case <-tk.C:
			s.us.Reaper(idle)
		}
	}
}

func (s *Server) clear() {
	if !s.running.Load() || s.uc == nil {
		return
	}

	s.running.Store(false)
	close(s.reapStop)

	s.wg.Wait()

	_ = s.uc.Close()
	s.uc = nil
	close(s.stopped)

	// 触发 Hook
	task := func() { s.hook.OnStop(s) }
	if !s.pool.Submit(task) {
		s.log.Error("fail to submit task: %v", task)
	}
}
