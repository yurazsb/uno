package conn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/yurazsb/uno/internal/boot"
	"github.com/yurazsb/uno/internal/conf"
	"github.com/yurazsb/uno/internal/decoder"
	"github.com/yurazsb/uno/internal/encoder"
	"github.com/yurazsb/uno/internal/framer"
	"github.com/yurazsb/uno/internal/handler"
	"github.com/yurazsb/uno/internal/hook"
	"github.com/yurazsb/uno/pkg/attrs"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const AcceptTimeout time.Duration = 2 * time.Second
const ReadTimeout time.Duration = 2 * time.Second
const WriteTimeout time.Duration = 30 * time.Second

// Transport 传输层策略
type Transport interface {
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Write(c *Conn, buf []byte) error // 单帧写
	Start(c *Conn)                   // 启动
	Stop(c *Conn)                    // 资源收尾
}

// Conn 会话层
type Conn struct {
	T Transport

	Id         string
	Local      net.Addr
	Remote     net.Addr
	Attributes boot.Attrs

	Cfg  *conf.Config
	Pool boot.Pool
	Log  boot.Logger

	Hook hook.ConnHook

	Ctx    context.Context
	Cancel context.CancelFunc

	Wg *sync.WaitGroup

	closed chan struct{}

	sendCh chan []byte

	framer  framer.Framer
	decoder decoder.Decoder
	encoder encoder.Encoder

	chain *handler.Chain

	rm      sync.Mutex
	readBuf bytes.Buffer

	startOnce sync.Once
	closeOnce sync.Once

	active atomic.Bool
	last   atomic.Int64
}

func NewConn(ctx context.Context, t Transport, cfg *conf.Config, hook hook.ConnHook) *Conn {
	c := &Conn{
		T:          t,
		Cfg:        cfg,
		Hook:       hook,
		Wg:         new(sync.WaitGroup),
		sendCh:     make(chan []byte, 10_000),
		closed:     make(chan struct{}),
		Attributes: attrs.New[any, any](true),
	}

	c.Id = cfg.IDGenerator()
	c.Local = t.LocalAddr()
	c.Remote = t.RemoteAddr()
	c.Ctx, c.Cancel = context.WithCancel(ctx)
	c.Pool = cfg.Pool
	c.Log = cfg.Logger
	c.framer = cfg.Framer
	c.decoder = cfg.Decoder
	c.encoder = cfg.Encoder
	c.chain = handler.NewChain(cfg.Handlers...)
	c.chain.Use(func(ctx handler.Context, next func()) {
		hook.OnMessage(ctx.Conn(), ctx.Payload())
	})

	return c
}

// ---- 对外基础接口 ----

func (c *Conn) ID() string                   { return c.Id }
func (c *Conn) Context() context.Context     { return c.Ctx }
func (c *Conn) LocalAddr() net.Addr          { return c.Local }
func (c *Conn) RemoteAddr() net.Addr         { return c.Remote }
func (c *Conn) Attrs() attrs.Attrs[any, any] { return c.Attributes }
func (c *Conn) IsActive() bool               { return c.active.Load() }

func (c *Conn) Send(msg any) error {
	if !c.IsActive() {
		return net.ErrClosed
	}

	buf, err := c.encoder(c, msg)
	if err != nil {
		return fmt.Errorf("encoder error: %w", err)
	}

	select {
	case <-c.Ctx.Done():
		return net.ErrClosed
	case c.sendCh <- buf:
		c.dispatchSend(msg)
		return nil
	}
}

func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		c.Cancel()
		select {
		case <-c.closed:
		case <-time.After(5 * time.Second):
			c.Log.Warn("force close conn %s", c.Id)
		}
	})
}

// ---- 内部开放接口 ----

func (c *Conn) Touch()                { c.last.Store(time.Now().UnixNano()) }
func (c *Conn) LastActive() time.Time { return time.Unix(0, c.last.Load()) }

func (c *Conn) SubmitTask(task func()) {
	ok := c.Pool.Submit(task)
	if !ok {
		c.dispatchError(fmt.Errorf("fail to submit task: %v", task))
	}
}

func (c *Conn) Recv(chunk []byte) {
	c.rm.Lock()
	defer c.rm.Unlock()

	// 刷新活跃时间
	c.Touch()

	// 追加到粘包缓冲
	if _, err := c.readBuf.Write(chunk); err != nil {
		c.dispatchRead(bytes.Clone(chunk), fmt.Errorf("read buffer write error: %w", err))
		return
	}

	// 拆帧
	frames, rest, err := c.framer(c, c.readBuf.Bytes())
	if err != nil {
		c.dispatchRead(bytes.Clone(chunk), fmt.Errorf("framer error: %w", err))
		return
	}

	c.dispatchRead(bytes.Clone(chunk), nil)

	// 适度回收：若缓冲非常大且剩余很小，重建缓冲以释放内存
	const shrinkFactor = 4
	if c.readBuf.Len() > c.Cfg.ReadBufferSize*shrinkFactor && len(rest) < c.Cfg.ReadBufferSize {
		var nb bytes.Buffer
		nb.Grow(len(rest))
		_, _ = nb.Write(rest)
		c.readBuf = nb
	} else {
		c.readBuf.Reset()
		_, _ = c.readBuf.Write(rest)
	}

	// 解码 & 派发
	for _, frame := range frames {
		fr := frame // 避免闭包变量复用
		c.SubmitTask(func() {
			msg, err := c.decoder(c, fr)
			if err != nil {
				c.dispatchError(fmt.Errorf("decoder error: %w", err))
				return
			}

			func() {
				defer func() {
					if r := recover(); r != nil {
						c.dispatchError(fmt.Errorf("handler process panic: %v", r))
					}
				}()

				c.chain.Handler(handler.NewContext(c, msg))
			}()
		})
	}
}

func (c *Conn) Start(wg *sync.WaitGroup) {
	c.startOnce.Do(func() {
		go c.mainLoop(wg) // 开始主循环
		go c.writeLoop()  // 开启写循环
		c.T.Start(c)      // 启动传输层

		c.active.Store(true)
		c.Touch()

		c.dispatchConnect()
	})
}

// mainLoop 连接主要工作循环，处理连接状态
func (c *Conn) mainLoop(wg *sync.WaitGroup) {
	wg.Add(1)
	defer func() { // 最终结束处理
		wg.Done()         // 结束当前占用的外部WG
		c.T.Stop(c)       // 结束传输层
		c.dispatchClose() // 触发关闭回调
		close(c.closed)   // 触发关闭通道
	}()

	var tickCh <-chan time.Time
	if c.Cfg.TickInterval > 0 {
		ticker := time.NewTicker(c.Cfg.TickInterval)
		defer ticker.Stop()
		tickCh = ticker.C
	}

	var idleNotified bool
	var idleCh <-chan time.Time
	if c.Cfg.IdleTimeout > 0 {
		idle := time.NewTicker(c.Cfg.IdleTimeout)
		defer idle.Stop()
		idleCh = idle.C
	}

	for {
		select {
		case <-c.Ctx.Done():
			close(c.sendCh) // 关闭消息队列
			c.Wg.Wait()     // 等他其他工作线程结束
			return
		case <-tickCh:
			c.dispatchTick()
		case <-idleCh:
			last := time.Unix(c.last.Load(), 0)
			if time.Since(last) > c.Cfg.IdleTimeout {
				if !idleNotified {
					idleNotified = true
					c.dispatchIdle()
				}
			} else {
				idleNotified = false
			}
		}

	}
}

func (c *Conn) writeLoop() {
	c.Wg.Add(1)
	defer c.Wg.Done()

	for buf := range c.sendCh {
		err := c.T.Write(c, buf)

		c.Touch()                 //刷新获取时间
		c.dispatchWrite(buf, err) //调用写入回调

		// 底层连接已关闭 结束循环
		if errors.Is(err, net.ErrClosed) {
			// 尝试 drain 剩余数据再退出
			for range c.sendCh {
			}
			c.Cancel() // 触发关闭信号
			break
		}
	}
}

// ---- Hook 映射 ----
func (c *Conn) dispatchConnect()        { c.SubmitTask(func() { c.Hook.OnConnect(c) }) }
func (c *Conn) dispatchClose()          { c.SubmitTask(func() { c.Hook.OnClose(c) }) }
func (c *Conn) dispatchError(err error) { c.SubmitTask(func() { c.Hook.OnError(c, err) }) }
func (c *Conn) dispatchTick()           { c.SubmitTask(func() { c.Hook.OnTick(c) }) }
func (c *Conn) dispatchIdle()           { c.SubmitTask(func() { c.Hook.OnIdle(c) }) }
func (c *Conn) dispatchSend(msg any)    { c.SubmitTask(func() { c.Hook.OnSend(c, msg) }) }
func (c *Conn) dispatchWrite(buf []byte, err error) {
	c.SubmitTask(func() { c.Hook.OnWrite(c, buf, err) })
}
func (c *Conn) dispatchRead(buf []byte, err error) {
	c.SubmitTask(func() { c.Hook.OnRead(c, buf, err) })
}
func (c *Conn) dispatchMessage(msg any) { c.SubmitTask(func() { c.Hook.OnMessage(c, msg) }) }
