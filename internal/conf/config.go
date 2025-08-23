package conf

import (
	"github.com/yurazsb/uno/internal/boot"
	"github.com/yurazsb/uno/internal/decoder"
	"github.com/yurazsb/uno/internal/encoder"
	"github.com/yurazsb/uno/internal/framer"
	"github.com/yurazsb/uno/internal/handler"
	"github.com/yurazsb/uno/pkg/logger"
	"github.com/yurazsb/uno/pkg/pool"
	"github.com/yurazsb/uno/pkg/uuid"
	"runtime"
	"time"
)

// Config 定义了通信框架的运行时配置。
// 建议通过 WithXXX 方法链式构建，再调用 WithDefault() 补齐未设置项。
type Config struct {
	// Pool 用于执行事件回调与任务分发的协程池。
	// 如果为 nil，会创建默认的高性能池（Hybrid 模式）。
	Pool boot.Pool

	// Logger 用于框架日志输出。
	// 如果为 nil，默认使用 logx.Default。
	Logger boot.Logger

	// Framer 用于消息的帧切分（FrameDecoder）。
	// 如果为 nil，默认使用内置的长度前缀协议。
	Framer framer.Framer

	// Decoder 将二进制数据解码为消息对象。
	// 如果为 nil，默认使用内置解码器。
	Decoder decoder.Decoder

	// Encoder 将消息对象编码为二进制数据。
	// 如果为 nil，默认使用内置编码器。
	Encoder encoder.Encoder

	// Handlers 注册的全局处理器链（中间件 / 事件分发器）。
	// 按照顺序执行。
	Handlers []handler.Handler

	// Network 网络类型（"tcp"、"udp"）。
	// 如果为空，默认使用 "tcp"。
	Network string

	// LocalAddr 服务连接的本地地址，例如 ":8080"。
	// 仅客户端有效
	LocalAddr string

	// IDGenerator 用于生成连接唯一 ID。
	// 如果为 nil，默认使用 NanoID(10)。
	IDGenerator func() string

	// NoDelay 是否启用 TCP_NODELAY（禁用 Nagle 算法）。
	// 仅TCP 服务端或客户端支持
	NoDelay bool

	// KeepAlive 是否启用 TCP keepalive。
	// 仅TCP 服务端或客户端支持
	KeepAlive bool

	// KeepAlivePeriod TCP keepalive 探测间隔。
	// 仅TCP 服务端或客户端支持 如果为 0，默认 2 分钟
	KeepAlivePeriod time.Duration

	// WriteTimeout 单次写操作的超时时间。
	// 如果为 0，使用框架可能设有兜底值 30s。
	WriteTimeout time.Duration

	// ReadBufferSize 读缓冲区大小（单位：字节）。
	// 如果为 0，默认 4096。
	ReadBufferSize int

	// MTU 最大传输单元，主要用于 UDP 分包场景。
	// 如果为 0，默认 1472。
	MTU int

	// IdleTimeout 连接空闲超时,UDP服务端使用伪连接，触发空闲超时会关闭清理连接。
	// 如果为 0，表示不启用空闲检测 UDP服务端默认为5分钟。
	IdleTimeout time.Duration

	// TickInterval 内部定时任务的周期（如 Idle 检测）。
	// 如果为 0，表示不启用周期任务。
	TickInterval time.Duration
}

func (c *Config) WithDefault() {
	if c.Pool == nil {
		c.Pool = pool.New(
			pool.WithMaxWorkers(runtime.GOMAXPROCS(0)*8),
			pool.WithQueue(8192),
			pool.WithIdleTimeout(30*time.Second),
			pool.WithNonBlocking(),
			pool.WithPanicHandler(func(r any) {
				c.Logger.Error("pool task panic: %v", r)
			}))
	}
	if c.Logger == nil {
		c.Logger = logger.Default("uno", logger.DEBUG)
	}
	if c.Framer == nil {
		c.Framer = framer.RawFramer()
	}
	if c.Decoder == nil {
		c.Decoder = decoder.RawDecoder()
	}
	if c.Encoder == nil {
		c.Encoder = encoder.GenericEncoder()
	}
	if c.Handlers == nil {
		c.Handlers = []handler.Handler{}
	}
	if c.Network == "" {
		c.Network = "tcp"
	}
	if c.IDGenerator == nil {
		c.IDGenerator = func() string { return uuid.NanoID(10) }
	}
	if c.KeepAlivePeriod <= 0 {
		c.KeepAlivePeriod = 2 * time.Minute
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 30 * time.Second
	}
	if c.ReadBufferSize <= 0 {
		c.ReadBufferSize = 4096
	}
	if c.MTU <= 0 {
		c.MTU = 1472
	}
}
