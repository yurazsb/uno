package tcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"uno/internal/boot"
	"uno/internal/boot/conn"
	"uno/internal/conf"
	"uno/internal/hook"
)

type Server struct {
	address string
	addr    net.Addr
	ln      net.Listener

	ctx    context.Context
	cancel context.CancelFunc

	cfg  *conf.Config
	pool boot.Pool
	log  boot.Logger

	hook hook.ServerHook

	running atomic.Bool

	started atomic.Bool

	stopOnce sync.Once
	stopped  chan struct{}

	wg *sync.WaitGroup
}

func NewServer(parent context.Context, cfg conf.Config, hook hook.ServerHook, addr string) *Server {
	ctx, cancel := context.WithCancel(parent)
	return &Server{
		address: addr,
		ctx:     ctx,
		cancel:  cancel,
		cfg:     &cfg,
		pool:    cfg.Pool,
		log:     cfg.Logger,
		hook:    hook,
		wg:      &sync.WaitGroup{},
		stopped: make(chan struct{}),
	}
}

func (s *Server) Addr() net.Addr { return s.addr }

func (s *Server) Context() context.Context { return s.ctx }

func (s *Server) IsRunning() bool { return s.running.Load() }

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

	s.running.Store(true)
	ln, err := net.Listen(s.cfg.Network, s.address)
	if err != nil {
		return err
	}

	s.ln = ln
	s.addr = ln.Addr()
	s.log.Debug("listening on %s://%s", s.cfg.Network, s.addr.String())

	task := func() { s.hook.OnStart(s) }
	if !s.pool.Submit(task) {
		s.log.Error("fail to submit task: %v", task)
	}

	return s.serve()
}

func (s *Server) serve() error {
	defer s.clear()

	listener, _ := s.ln.(*net.TCPListener)
	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
			_ = listener.SetDeadline(time.Now().Add(conn.AcceptTimeout))
			raw, err := s.ln.Accept()
			if err != nil {
				// 处理 err（优先级顺序: 本端主动关闭 > 超时 / 临时 > 其他）
				if errors.Is(err, net.ErrClosed) {
					// 本端主动关闭（预期），直接退出
					return nil
				}

				// net.Error (超时 / 临时)
				var nerr net.Error
				if errors.As(err, &nerr) {
					if nerr.Timeout() {
						// 读超时（deadline 到期）继续循环
						continue
					}

					if nerr.Temporary() {
						// 临时错误（网络抖动）继续循环
						continue
					}
				}

				// 其他错误 直接退出 结束服务
				s.log.Warn("accept error: %s", err)
				return err
			}

			nc := conn.NewNETConn(s.ctx, raw, s.cfg, s.hook)
			nc.Start(s.wg)
		}
	}
}

func (s *Server) clear() {
	if !s.running.Load() || s.ln == nil {
		return
	}

	s.running.Store(false)

	_ = s.ln.Close()
	s.ln = nil

	s.wg.Wait()

	task := func() { s.hook.OnStop(s) }
	if !s.pool.Submit(task) {
		s.log.Error("fail to submit task: %v", task)
	}

	close(s.stopped)
}
