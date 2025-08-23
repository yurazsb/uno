package uno

import (
	"context"
	"fmt"
	"github.com/yurazsb/uno/internal/boot"
	"github.com/yurazsb/uno/internal/boot/tcp"
	"github.com/yurazsb/uno/internal/boot/udp"
	"github.com/yurazsb/uno/internal/conf"
	"github.com/yurazsb/uno/internal/decoder"
	"github.com/yurazsb/uno/internal/encoder"
	"github.com/yurazsb/uno/internal/framer"
	"github.com/yurazsb/uno/internal/handler"
	"github.com/yurazsb/uno/internal/hook"
	"time"
)

type Server = boot.Server
type Client = boot.Client
type Conn = boot.Conn

type Attrs = boot.Attrs
type Pool = boot.Pool
type Logger = boot.Logger

type ServerHook = hook.ServerHook
type ConnHook = hook.ConnHook
type ServerEvent = hook.ServerEvent
type ConnEvent = hook.ConnEvent

type Framer = framer.Framer

var RawFramer = framer.RawFramer
var LineFramer = framer.LineFramer
var DelimiterFramer = framer.DelimiterFramer
var FixedLengthFramer = framer.FixedLengthFramer
var LengthFieldFramer = framer.LengthFieldFramer

type Decoder = decoder.Decoder

var RawDecoder = decoder.RawDecoder
var StringDecoder = decoder.StringDecoder

type Encoder = encoder.Encoder

var GenericEncoder = encoder.GenericEncoder

type Handler = handler.Handler
type Context = handler.Context

var RateLimitHandler = handler.RateLimitHandler

var RouterHandler = handler.RouterHandler
var NewRouter = handler.NewRouter

type Router = handler.Router
type RouterGroup = handler.RouterGroup
type Route = handler.Route

type Config = conf.Config
type Option = func(*Config)

// WithPool 设置协程池
func WithPool(p Pool) Option {
	return func(c *Config) {
		c.Pool = p
	}
}

// WithLogger 设置日志器
func WithLogger(l Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithFramer 设置消息帧解析器
func WithFramer(f Framer) Option {
	return func(c *Config) {
		c.Framer = f
	}
}

// WithDecoder 设置消息解码器
func WithDecoder(d Decoder) Option {
	return func(c *Config) {
		c.Decoder = d
	}
}

// WithEncoder 设置消息编码器
func WithEncoder(e Encoder) Option {
	return func(c *Config) {
		c.Encoder = e
	}
}

// WithHandlers 设置全局处理器链
func WithHandlers(Handlers ...Handler) Option {
	return func(c *Config) {
		c.Handlers = Handlers
	}
}

// WithNetwork 设置网络类型 (tcp/udp)
func WithNetwork(n string) Option {
	return func(c *Config) {
		c.Network = n
	}
}

// WithLocalAddr 设置本地地址，仅客户端有效
func WithLocalAddr(addr string) Option {
	return func(c *Config) {
		c.LocalAddr = addr
	}
}

// WithIDGenerator 设置连接唯一 ID 生成器
func WithIDGenerator(gen func() string) Option {
	return func(c *Config) {
		c.IDGenerator = gen
	}
}

// WithNoDelay 设置 TCP_NODELAY
func WithNoDelay(nd bool) Option {
	return func(c *Config) {
		c.NoDelay = nd
	}
}

// WithKeepAlive 设置 TCP KeepAlive
func WithKeepAlive(ka bool) Option {
	return func(c *Config) {
		c.KeepAlive = ka
	}
}

// WithKeepAlivePeriod 设置 TCP KeepAlive 探测间隔
func WithKeepAlivePeriod(period time.Duration) Option {
	return func(c *Config) {
		c.KeepAlivePeriod = period
	}
}

// WithWriteTimeout 设置单次写操作超时
func WithWriteTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.WriteTimeout = timeout
	}
}

// WithReadBufferSize 设置读缓冲区大小
func WithReadBufferSize(size int) Option {
	return func(c *Config) {
		c.ReadBufferSize = size
	}
}

// WithMTU 设置最大传输单元
func WithMTU(mtu int) Option {
	return func(c *Config) {
		c.MTU = mtu
	}
}

// WithIdleTimeout 设置连接空闲超时
func WithIdleTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.IdleTimeout = timeout
	}
}

// WithTickInterval 设置内部定时任务周期
func WithTickInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.TickInterval = interval
	}
}

// initConfig 初始化配置
func initConfig(opts ...Option) conf.Config {
	cfg := conf.Config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	// 设置默认配置
	cfg.WithDefault()
	return cfg
}

// Serve 启动一个 TCP 或 UDP 服务，阻塞运行直到 Stop() 调用或出错
func Serve(ctx context.Context, hook hook.ServerHook, addr string, opts ...Option) error {
	cfg := initConfig(opts...)

	// 启动服务
	switch cfg.Network {
	case "tcp", "tcp4", "tcp6":
		srv := tcp.NewServer(ctx, cfg, hook, addr)
		return srv.Listen()
	case "udp", "udp4", "udp6":
		srv := udp.NewServer(ctx, cfg, hook, addr)
		return srv.Listen()
	default:
		return fmt.Errorf("unknown network: %s", cfg.Network)
	}
}

// Dial 连接一个 TCP 或 UDP 服务
func Dial(ctx context.Context, hook hook.ConnHook, addr string, opts ...Option) (boot.Conn, error) {
	cfg := initConfig(opts...)

	// 创建客户端
	var c boot.Client
	switch cfg.Network {
	case "tcp", "tcp4", "tcp6":
		c = tcp.NewClient(ctx, cfg, hook, addr)
	case "udp", "udp4", "udp6":
		c = udp.NewClient(ctx, cfg, hook, addr)
	default:
		return nil, fmt.Errorf("unknown network: %s", cfg.Network)
	}

	// 连接
	return c.Dial()
}
