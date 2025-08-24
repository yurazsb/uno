### 开始

#### 简介

**uno** 是一套面向高并发与扩展性的 **TCP/UDP 通信框架**。意在以 **统一抽象** 将「连接管理、流式帧解析（Framer）、消息编解码（Codec）、中间件链（Middleware）、路由分发（Dispatcher）」无缝串联，让你专注在 **业务协议与处理逻辑**，而不是底层网络细节。

**核心特性一览：**

- **统一模型：** TCP 与 UDP（伪连 接）共享一致的 `Server / Client / Conn` 抽象与事件回调（`OnConnect / OnMessage / OnClose`等）。
- **可插拔协议栈：** Framer 负责分帧、Codec 负责序列化，二者解耦；支持内置实现与自定义扩展。
- **中间件链：** 类似 HTTP 的拦截器链，内置或自定义日志、鉴权、限流（RateLimiter）等能力；支持路由分发（Dispatcher）。
- **连接生命周期管理：** 连接属性存储、心跳/空闲检测、读写超时、优雅关闭与错误恢复。
- **事件驱动高并发：** 读/写/控制三通道并发模型，配合可选 Goroutine 池，稳定处理突发流量。
- **易于上手与维护：** 清晰的包结构与接口边界，示例齐全，适合二次开发与定制。
- `uno` 完全基于 Go 标准库构建，未引入任何第三方依赖。

**典型场景：**

- IoT 设备接入与网关
- 游戏与实时对战/匹配服务
- IM/推送/实时通知
- 自定义二进制/文本协议的专有服务

**数据流向（简图）：**

```text
写出：Send → Encoder → Socket Write

接收：Socket Read → Framer → Decoder → Handler Chain → OnMessage
                              			  ↓
                       			(限流、路由或自定义Handler)
```

用 **uno**，你可以以最少的样板代码构建稳定、可观测、可演进的网络服务。

---

#### 快速开始
##### 安装

```sh
go get github.com/yurazsb/uno@v0.1.3
```



##### 服务端

示例：Echo Server

```go
type EchoServer struct {
	uno.ServerEvent // 内嵌默认空实现
}

// 收到客户端消息时回调
func (e *EchoServer) OnMessage(c core.Conn, msg any) {
	log.Printf("OnMessage %s %v %s", c.RemoteAddr().String(), msg, string(msg.([]byte)))
    
	// 回发给客户端
    _ = c.Send("hello client!")
}

func main() {
    // 网络类型：tcp、tcp4、tcp6、udp、udp4、udp6
	network := "tcp"
    
    // 启动服务端监听 :9090
	err := uno.Serve(context.Background(), &EchoServer{}, ":9090", uno.WithNetwork(network))
	if err != nil {
		fmt.Println(err)
	}
}
```

运行后，服务端会在 `:9090` 启动，并在收到消息时回显 `"hello client!"`。

##### 客户端

示例：Echo Client

```go
type EchoClient struct {
	uno.ConnEvent // 内嵌默认空实现
}

// 收到服务端消息时回调
func (e *EchoClient) OnMessage(c core.Conn, msg any) {
	log.Printf("OnMessage %s %v %s", c.RemoteAddr().String(), msg, string(msg.([]byte)))
}

func main() {
    // 网络类型：tcp、tcp4、tcp6、udp、udp4、udp6
	network := "tcp" 
    
    // 连接服务端
	conn, err := uno.Dial(context.Background(), &EchoClient{}, "127.0.0.1:9090", uno.WithNetwork(network))
	if err != nil {
		fmt.Println(err)
	}

    // 每隔 2 秒向服务端发送一条消息
	tick := time.NewTicker(time.Second * 2)
	defer tick.Stop()
	for range tick.C {
		_ = conn.Send("hello server!")
	}
}

```

运行客户端后，你会在服务端日志中看到来自客户端的消息，同时客户端也会收到服务端回发的 `"hello client!"`。

---

### 基础

#### 事件回调 

`uno` 框架提供了 **事件回调接口**，用于在服务端和连接生命周期中注入自定义逻辑。
 主要分为 **Server 级别回调** 和 **Conn（连接）级别回调**。

##### ServerHook

`ServerHook` 用于监听服务端整体生命周期事件，并包含连接事件回调。

```go
type ServerHook interface {
    OnStart(s Server)     // 服务启动时触发
    OnStop(s Server)      // 服务停止时触发
    ConnHook              // 嵌套连接回调接口
}
```

##### ConnHook

`ConnHook` 用于监听单个连接的生命周期和消息事件。

```go
type ConnHook interface {
    OnConnect(c Conn)                // 连接建立时触发
    OnClose(c Conn)                  // 连接关闭时触发
    OnError(c Conn, err error)       // 连接发生错误时触发
    OnTick(c Conn)                   // 周期性任务触发时调用
    OnIdle(c Conn)                   // 连接空闲时触发
    OnSend(c Conn, msg any)          // 消息发送前触发
    OnWrite(c Conn, buf []byte, err error) // 写操作完成后触发
    OnRead(c Conn, buf []byte, err error)  // 读操作完成后触发
    OnMessage(c Conn, msg any)       // 收到消息后触发
}
```

##### 使用示例

框架提供了 **ServerEvent** 和 **ConnEvent** 作为空实现，可按需重写部分方法：

```
// 自定义事件回调
type MyHook struct {
    uno.ServerEvent
} 

func (h *MyHook) OnStart(s Server) {
    fmt.Println("Server started!")
}

func (h *MyHook) OnConnect(c Conn) {
    fmt.Println("New connection:", c.ID())
}

func (h *MyHook) OnMessage(c Conn, msg any) {
    fmt.Println("Received message:", msg)
    // 发送回执
    c.Send("ack")
}
```

注册到服务：

```go
uno.Serve(context.Background(), &MyHook{}, ":9090")
// 或
uno.Dail(context.Background(), &MyHook{}, "127.0.0.1:9090")
```

> 通过继承 **ServerEvent / ConnEvent**，你只需关注感兴趣的回调，代码清晰简洁。

---

#### 配置

`uno` 的 **Config** 定义了通信框架的运行时行为，包括网络类型、协程池、日志、帧解析器、编码器/解码器、全局中间件等。

| 字段            | 默认值                            | 说明                       |
| --------------- | --------------------------------- | -------------------------- |
| Pool            | 高性能协程池（Hybrid 模式）       | 用于事件回调和任务分发     |
| Logger          | `logx.Default("uno", logx.DEBUG)` | 日志输出器                 |
| Framer          | 默认长度前缀帧                    | 用于消息切分               |
| Decoder         | 默认解码器                        | 将二进制数据解码为消息对象 |
| Encoder         | 默认编码器                        | 将消息对象编码为二进制数据 |
| Handlers        | 空链                              | 全局中间件链               |
| Network         | `"tcp"`                           | 网络类型                   |
| IDGenerator     | NanoID(10)                        | 生成连接唯一 ID            |
| KeepAlivePeriod | 2 分钟                            | TCP keepalive 探测间隔     |
| WriteTimeout    | 30 秒                             | 单次写操作超时             |
| ReadBufferSize  | 4096 字节                         | 读缓冲区大小               |
| MTU             | 1472 字节                         | UDP 最大传输单元           |
| IdleTimeout     | 0 或 UDP 服务端伪连接默认 5 分钟  | 空闲连接超时               |
| TickInterval    | 0                                 | 内部定时任务周期           |

> 通过 **`WithXXX` 方法**构建配置

```go
func main() {
    // 网络类型：tcp、tcp4、tcp6、udp、udp4、udp6
	network := "tcp"
    
    // 启动服务端监听 :9090
	err := uno.Serve(context.Background(), &EchoServer{}, ":9090", uno.WithNetwork(network)...)
	if err != nil {
		fmt.Println(err)
	}
}
```

---

#### 流式帧解析

在 TCP/UDP 等面向字节流的协议中，**消息可能被拆分或粘包**，需要对接收到的字节流进行拆分，得到完整消息帧。`uno` 框架通过 **Framer 接口**提供统一的拆帧机制。

**运行时位置**：Socket Read → `Framer `→ Decoder → Handler Chain→ OnMessage

##### 函数签名

```go
type Framer func(c Conn, buf []byte) (frames [][]byte, remaining []byte, err error)
```

**参数说明：**

| 参数  | 类型     | 说明                                         |
| ----- | -------- | -------------------------------------------- |
| `c`   | `Conn`   | 当前连接对象，可用于获取连接状态或上下文信息 |
| `buf` | `[]byte` | 接收到的原始字节流                           |

**返回值：**

| 返回值      | 类型       | 说明                                         |
| ----------- | ---------- | -------------------------------------------- |
| `frames`    | `[][]byte` | 从字节流中解析出的完整消息帧列表             |
| `remaining` | `[]byte`   | 未能形成完整消息的剩余字节，保留到下一次解析 |
| `err`       | `error`    | 拆帧过程中出现的错误                         |

##### 常用实现

###### RawFramer

```go
// RawFramer 返回一个“直通式”的帧解码器 (Framer) 默认使用该Framer。
//
// 该解码器不会对输入字节流进行任何拆分，而是将整个 buf 作为一条完整帧返回。
// 通常用于：
//   - 协议本身无需拆帧（例如 datagram/UDP、一次性读取完整数据）。
//   - 测试或调试场景，用于直接观察原始数据。
//   - 上层业务逻辑自行处理数据切分。
//
// 参数说明：
//   - 无（内部固定实现）。
//
// 使用场景：
//   - 无需拆包的协议（UDP、一次性短连接 TCP）。
//   - 调试/开发阶段，快速查看接收数据。
//
// 协议示例：
//
//	输入字节流: "HelloWorld"
//	解码结果: ["HelloWorld"]
//	剩余: nil
func RawFramer() Framer {...}
```

###### FixedLengthFramer

```go
// FixedLengthFramer 返回一个基于“固定长度”的帧解码器 (Framer)。
//
// 该解码器假设每条消息的长度固定为 length 字节。
// 在解析过程中，会不断从 buf 中切割出定长帧，直到剩余数据不足一帧为止。
// 剩余未满一帧的数据将存入 remaining，以便下次拼接。
//
// 参数说明：
//   - length: 每个消息帧的固定字节数（必须大于 0）。
//
// 使用场景：
//   - 固定包长的二进制协议或嵌入式设备通信协议。
//   - 游戏服务器/IoT 设备中部分简化协议。
//   - 日志采集、传感器数据、简易传输格式。
//
// 协议示例：
//
//  输入字节流: "ABCDEFGHIJK"
//  配置: FixedLengthFramer(3)
//  解码过程:
//    → "ABC"
//    → "DEF"
//    → "GHI"
//  剩余: "JK"
//  （等待后续拼接两个字节，组成下一条完整消息 "JKX"...）
func FixedLengthFramer(length int) Framer {...}
```

###### DelimiterFramer

```go
// DelimiterFramer 返回一个基于“分隔符”的帧解码器 (Framer)。
//
// 该解码器通过指定的分隔符 (delim) 将字节流拆分为多帧。
// 每当遇到分隔符时，之前的数据段会作为一帧返回，分隔符本身会被丢弃。
// 剩余未遇到分隔符的数据将存入 remaining，以便下次拼接继续解析。
//
// 参数说明：
//   - delim: 消息之间的分隔符。例如 "\n"、"\r\n"、"\0"。
//
// 使用场景：
//   - 文本协议（Telnet、Redis RESP、SMTP、POP3 等）。
//   - 行协议、命令式协议、日志传输。
//   - 简单消息协议，以换行或其他字符作为结束符。
//
// 协议示例：
//
//  输入字节流: "PING\nPONG\nPARTIAL"
//  分隔符: "\n"
//  解码结果: ["PING", "PONG"]
//  剩余: "PARTIAL"  (等待下次拼接)
func DelimiterFramer(delim []byte) Framer {...}
```

###### LineFramer

```go
// LineFramer 返回一个基于换行符("\n")的帧解码器。
//
// 这是 DelimiterFramer 的特化版本，使用 "\n" 作为分隔符，
// 每一行数据对应一条消息帧（不包含换行符本身）。
//
// 使用场景：
//   - 逐行读取的文本协议：SMTP、POP3、HTTP 请求行、IRC 协议。
//   - 日志/监控数据流：一行一条记录。
//
// 协议示例：
//
//  输入字节流: "HELLO\nWORLD\nTEST"
//  解码结果: ["HELLO", "WORLD"]
//  剩余: "TEST"
//
// 等待后续输入拼接 "TEST\n" 后，得到完整帧 "TEST"。
func LineFramer() Framer {
    return DelimiterFramer([]byte("\n"))
}
```

###### LengthFieldFramer

```go
// LengthFieldFramer 返回一个基于“长度字段”的帧解码器 (Framer)。
//
// 该解码器假设消息包格式为：
//
//  [header | length field | payload]
//
// 即在包头的某个偏移位置存在一个长度字段，标识后续有效负载(payload)的长度。
//
// 函数会从输入的 buf 中不断解析出完整帧，直到 buf 不足以构成一个完整包。
// 解析出的帧数据将组成 frames 返回，剩余未处理的半包会放入 remaining 中，以便下一次拼接。
//
// 参数说明：
//   - lengthFieldOffset:     长度字段在消息中的起始偏移（相对整个消息起始位置，单位：字节）。
//     例如：如果消息格式为 [2字节魔数][4字节长度][数据体]，则该值为 2。
//   - lengthFieldSize:       长度字段本身的字节数（1、2、4 或 8），即用多少字节表示数据长度。
//   - lengthAdjustment:      长度字段的修正值，用于调整实际 payload 的长度。
//     例如：某些协议长度字段表示全部数据体长度或部分payload长度，可以通过此值修正为后继 payload 实际长度。
//   - initialBytesToStrip:   从最终帧中需要剥离的前置字节数。常用于跳过协议头，仅保留 payload。
//     例如：设置为 lengthFieldOffset+lengthFieldSize，可以直接返回数据体部分。
//   - order:                 长度字段的字节序（binary.BigEndian 或 binary.LittleEndian）。
//
// 使用场景：
//   - 自定义二进制协议：常见于 TCP 协议栈或 RPC 框架。
//   - Protobuf/Thrift 封装：通常前 4 个字节为消息长度。
//   - Netty LengthFieldBasedFrameDecoder 的 Go 实现。
//
// 协议示例 1：简单长度字段协议
//
//  格式: [4字节长度][payload]
//  配置: LengthFieldFramer(0, 4, 0, 4, binary.BigEndian)
//  示例: 00 00 00 05  48 65 6C 6C 6F
//       |--len=5--|  H  e  l  l  o
//  解码结果: "Hello"
//
// 协议示例 2：带魔数与长度字段
//
//  格式: [2字节魔数][4字节长度][payload]
//  配置: LengthFieldFramer(2, 4, 0, 6, binary.BigEndian)
//  示例: 0xAB 0xCD  00 00 00 05  48 65 6C 6C 6F
//         magic    |--len=5--|   payload
//  解码结果: "Hello"
//
// 协议示例 3：长度字段包含包头长度
//
//  格式: [4字节长度(含头)][payload]
//  配置: LengthFieldFramer(0, 4, -4, 4, binary.BigEndian)
//  示例: 00 00 00 09  01 02 03 04 05
//       |---len=9 (4字节头+5字节体)--|
//  解码结果: 01 02 03 04 05
func LengthFieldFramer(lengthFieldOffset, lengthFieldSize, lengthAdjustment, initialBytesToStrip int, order binary.ByteOrder) Framer {...}
```



##### 使用方式

```go
uno.Serve(context.Background(), &uno.ServerEvent{}, ":9090", uno.WithFramer(uno.LineFramer()))
// 或
uno.Dail(context.Background(), &uno.ConnEvent{}, "127.0.0.1:9090", uno.WithFramer(uno.LineFramer()))
```

> 框架在读缓冲区中接收字节后，会调用 **Framer ** 对字节流进行拆帧，然后将每条完整消息交给 Decoder 和消息处理链。
>
> 如需复杂拆帧，请自定义 **Framer ** 

---

#### 解码器

**Decoder** 是用于将接收到的二进制数据解码为业务可用对象的接口，在**Framer**之后发挥作用，确保每次解码的数据都是完整帧。

**运行时位置**：Socket Read → Framer → `Decoder `→ Handler Chain→ OnMessage

##### 函数签名

```go
type Decoder func(c Conn, buf []byte) (msg any, err error)
```

**参数说明：**

| 参数  | 类型     | 说明                                         |
| ----- | -------- | -------------------------------------------- |
| `c`   | `Conn`   | 当前连接对象，可用于获取连接状态或上下文信息 |
| `buf` | `[]byte` | 单帧的原始字节切片（来源于 Framer）          |

**返回值：**

| 返回值 | 类型    | 说明                                                         |
| ------ | ------- | ------------------------------------------------------------ |
| `msg`  | `any`   | 上层可消费的消息对象（可为结构体、接口实现、轻量包装类型，或必要时仍为 []byte）。 |
| `err`  | `error` | 解码过程中出现的错误                                         |

##### 常用实现

###### RawDecoder

```go
// RawDecoder 创建一个原始二进制解码器 默认使用该Decoder。
// 它直接返回接收到的字节切片，不做任何解析或转换。
//
// 使用说明:
//   - 常用于不需要解码的二进制数据传输场景
//   - 可与任何 Framer 搭配使用
func RawDecoder() Decoder {...}
```

###### StringDecoder

```go
// StringDecoder 返回一个文本解码器，将字节切片转换为字符串。
// 可以选择是否去掉首尾空白字符。
//
// 参数:
//   - trim: 是否调用 strings.TrimSpace 去掉首尾空白
//
// 使用说明:
//   - 常用于文本协议（如 HTTP、命令行协议、日志协议）
//
// 注意事项:
//   - trim 参数为 true 时会去掉首尾空格、换行符等
//   - 不会对中间空格进行处理
func StringDecoder(trim bool) Decoder {...}
```

##### 使用方式

```go
uno.Serve(context.Background(), &uno.ServerEvent{}, ":9090", uno.WithDecoder(uno.StringDecoder()))
// 或
uno.Dail(context.Background(), &uno.ConnEvent{}, "127.0.0.1:9090", uno.WithDecoder(uno.StringDecoder()))
```

> 如需复杂解码，请自定义 **Decoder ** 

---

#### 处理链

Handler 是 `uno` 消息处理链中的核心组成部分，用于在 **消息到达** 时进行拦截、处理或分发。

**运行时位置**：Socket Read → Framer → Decoder → `Handler Chain ` → OnMessage

**作用**：

- 拦截与增强（日志、限流、鉴权等）
- 分发（路由到不同业务逻辑）
- 扩展（用户自定义功能）

##### 函数签名

```go
type Handler func(ctx Context, next func())
```

**参数说明：**

| 参数   | 类型   | 说明                                         |
| ------ | ------ | -------------------------------------------- |
| `c`tx  | `Context` | 封装了当前连接、消息与上下文的对象（详见 Context）。 |
| `next` |`Func`|调用下一个 Handler。如果不调用，执行链会在此中断。|

> 这种模式允许开发者 **拦截消息**，例如在限流时不调用 `next()`，从而终止消息继续传递。

**Context：**

| 方法                      | 说明                                                         |
| ------------------------- | ------------------------------------------------------------ |
| `Context()`               | 返回标准库 `context.Context`，支持超时、取消控制             |
| `Cancel()`                | 主动取消当前上下文，通常用于中止消息处理，仅用于终止信号，执行链是否终止取决于`next()` |
| `Done()` / `Err()`        | 与标准 `context.Context` 一致，用于检查是否已结束            |
| `Attrs()`                 | 获取连接级别的属性存储（KV 结构，可跨 Handler 共享数据）     |
| `Conn()`                  | 当前消息对应的连接对象                                       |
| `Payload()`               | 获取当前消息体（经过 Framer/Decoder 解码后的对象）           |
| `SetPayload(payload any)` | 设置/修改当前消息体，传递给后续 Handler                      |

---

##### 可用实现

###### 限流 

`RateLimitHandler` 是 Uno 框架内置的中间件，用于对连接和全局的消息处理速率进行限制。
 它基于 **令牌桶（Token Bucket）** 算法实现，既能控制消息的平均速率，又能容忍一定程度的突发流量，从而在高并发场景下保护服务器稳定运行。

该处理器可以作用于：

- **单连接**：限制某个连接的独立速率，防止“坏连接”刷爆服务器。

- **全局**：限制整个服务的总处理速率，保证系统整体不会过载

- **参数说明：**

  ```
  func RateLimitHandler(
      connRate   int64,                     				// 单连接每秒可处理的平均消息数
      connBurst  int64,                     				// 单连接允许的最大突发消息数
      globalRate int64,                     				// 全局每秒可处理的平均消息数
      globalBurst int64,                    				// 全局允许的最大突发消息数
      limit func(ctx core.Context, limit Handler),        // 被限流时的回调，同样也是个Handler
  ) Handler
  ```

  - **connRate**
     单连接的令牌补充速率（每秒消息数）。
     例如：`connRate = 10` → 每秒允许处理 10 条消息。
  - **connBurst**
     单连接桶容量（最大瞬时突发）。
     例如：`connRate = 10, connBurst = 20` → 平时速率 10/秒，但瞬时最多能承受 20 条。
  - **globalRate**
     全局速率限制（所有连接加起来）。
  - **globalBurst**
     全局桶容量（全局瞬时突发）。
  - **limit**
     当消息被限流时执行的`Handler`，例如可以打印日志或主动断开连接。

  **工作机制：**

  1. **令牌桶原理**
     - 每个桶以 `Rate` 的速率持续补充令牌，但不会超过 `Burst` 容量。
     - 每条消息到达时需要消耗一个令牌。
     - 如果桶里没有令牌 → 消息被丢弃或执行 `limit` 回调。
  2. **双层桶设计**
     - **单连接桶**：为每个连接分配一个独立的桶，防止单个连接刷爆服务器。
     - **全局桶**：所有连接共享一个全局桶，控制整体处理能力。
  3. **清理机制**
     - 框架会定期清理长时间不用的连接桶，避免内存泄漏。

---

###### 路由

`RouterHandler` 是 UNO 的核心 Handler 之一，它将 **TCP/UDP 消息分发** 抽象为类似 Web 框架的「路由系统」。
 通过 `RouterHandler`，你可以为不同的请求路径（或标识符）注册处理链（middleware），从而实现清晰的业务逻辑分发。

**参数说明**

```
// RouterHandler 返回一个路由分发 Handler。
// resolver 是一个函数，用于从 core.Context 中提取请求路径（或路由标识），
// router 路由器。
func RouterHandler(resolver func(ctx core.Context) (string, bool), router *Router) Handler
```

**工作流程：**

路由分发的完整逻辑：

1. 调用 `resolver(ctx)` 提取路径，若返回 `ok = false`，直接放行到 `next()`。
2. 若解析成功，调用 `router.Match(path)` 查找对应的路由。
3. 如果找到匹配的 `Route`：
   - 组合并执行该路由的 Handler 链。
4. 如果未匹配：
   - 执行 `Router.NotFound` 设置的处理链（如果存在）。

> 路由中的 Handler 链 与外部 Handler 链形成一条完整的处理链。

**Router：**

`Router` 是路由器核心，内部用 **Trie 树** 存储路径，同时配合可选的 **LRU 缓存** 来加速查询。

功能：

- **SplitPath/JoinPath**：路径切割与拼接。
- **Lru**：设置缓存大小。
- **NotFound**：配置未匹配的处理链。
- **Match**：查找路由并返回对应的 `Route`。

**RouterGroup：**

`RouterGroup` 提供了类似 Web 框架（如 Gin）的分组功能。

特点：

- 支持前缀继承：`/api` → `/api/v1`
- 支持层级中间件：外层组的中间件会在内层组前执行
- 提供 `Group`、`Use`、`Handle` 等接口

**Route：**

`Route` 表示单条具体路由，包含：

- **Path**：注册路径。
- **Handlers**：完整处理链（组中间件 + 路由自身 handler）。

**典型应用场景：**

+ **IoT 协议网关**：不同 topic/path → 不同设备处理链。
+ **游戏服务器**：消息号 / 动作名 → 业务逻辑。
+ **RPC 框架**：方法名 → RPC handler。

---

###### 使用方式

##### 使用方式

```go
limitHandler := uno.RateLimitHandler(10, 20, 100, 200, func(ctx core.Context, next func()) {
	fmt.Println("Traffic limit！！")
})

router := uno.NewRouter("/")
router.Handle("Login", func(ctx core.Context, next func()) {
	fmt.Println("Login")
})

var resolver = func(ctx core.Context) (string, bool) {
	path, ok := ctx.Attrs().Get("path")
	if !ok {
		return "", false
	}
	s, ok := path.(string)
		return s, ok
}

routerHandler := uno.RouterHandler(resolver, router)

uno.Serve(context.Background(), &uno.ServerEvent{}, ":9090", uno.WithHandlers(limitHandler, routerHandler))
// 或
uno.Dail(context.Background(), &uno.ConnEvent{}, "127.0.0.1:9090",uno.WithHandlers(limitHandler))
```

> 如需其他复杂处理，请自定义 **Handler** 。

---

#### 编码器 

**Encoder **是框架中用于将消息对象编码为字节切片的函数类型。在使用`conn.Send(any)`方法发送消息时，消息会被`Encoder`处理。

**运行时位置**：Send → `Encoder ` → Socket Write

##### 函数签名

```go
type Decoder func(c Conn, buf []byte) (msg any, err error)
```

**参数说明：**

| 参数 | 类型   | 说明                                                 |
| ---- | ------ | ---------------------------------------------------- |
| `c`  | `Conn` | 当前连接实例，可用于自定义编码逻辑（默认实现未使用） |
| msg  | any    | 待编码的消息，可以是任意类型                         |

**返回值：**

| 返回值 | 类型     | 说明               |
| ------ | -------- | ------------------ |
| `buf`  | `[]byte` | 编码后的字节切片   |
| `err`  | `error`  | 编码失败时返回错误 |

**GenericEncoder**

```go
// GenericEncoder 返回一个通用编码器 (Encoder)，默认使用该Encoder。
//
// 功能说明：
//   - 支持常用基础类型编码为字节切片，包括:
//     []byte, string, int/uint 系列, float32/64, bool
//   - 对其他类型 (struct、map、slice 等) 自动进行 JSON 序列化
//
// 使用场景：
//   - 适用于简单协议或通用消息编码
//   - 当消息类型多样且不固定时，可以直接使用 GenericEncoder
func GenericEncoder() Encoder {...}
```

##### 使用方式

```go
uno.Serve(context.Background(), &uno.ServerEvent{}, ":9090", uno.WithEncoder(uno.GenericEncoder()))
// 或
uno.Dail(context.Background(), &uno.ConnEvent{}, "127.0.0.1:9090", uno.WithEncoder(uno.GenericEncoder()))
```

> 如需复杂编码，请自定义 **Encoder** 。

---

### 架构设计

#### 概述

`uno` 是一个 **轻量级的 TCP/UDP 通信框架**，核心目标是简化网络编程中的重复工作，让开发者能够用极少的代码构建稳定、高效的网络应用。

在 Go 原生的 `net` 包中，开发者往往需要手动处理：

- 连接的生命周期管理（创建、关闭、异常处理）
- 消息的拆包与粘包问题（Framer）
- 消息编解码（Codec）
- 并发协程调度与资源回收
- 中间件能力（日志、限流、鉴权等）

这些工作本身和业务逻辑无关，却经常消耗开发者大量时间。`uno` 将这些部分抽象成统一接口，通过事件驱动模型对外暴露，使得开发者只需实现少量回调，就能专注于 **业务层逻辑**。

`uno` 完全基于 Go 标准库构建，未引入任何第三方依赖。
 这意味着：

- **开箱即用**：只需 `go get github.com/xxx/uno` 即可使用
- **无额外负担**：避免依赖膨胀和安全漏洞风险
- **可学习性强**：所有实现都基于原生库，便于理解和二次开发

---

**为什么要设计 `uno`**

设计 `uno` 有两个主要出发点：

1. **简化开发流程**
   - 统一 TCP/UDP 的抽象接口
   - 提供开箱即用的 Framer、Codec、Hook
   - 通过中间件链路扩展业务功能
      → 让开发者写一个 Echo 服务只需要几十行代码，而不是上百行循环和粘包处理逻辑。
2. **自我学习与实践**
   - 通过实现框架，深入掌握 Go 的并发模型、网络 IO、context 控制
   - 学习事件驱动和模块化设计思想
   - 对比成熟框架（如 Netty、gnet），总结轻量级框架设计的取舍

`uno` 因此既是一个实用的工具库，也是一个学习和研究网络编程的实验场。

---

**使用场景**

虽然 `uno` 本身定位轻量，但它适合应用在多种网络场景中：

- **IoT 设备接入服务**
  - 大量设备通过 TCP/UDP 接入，要求高并发、低资源占用
- **游戏服务器开发**
  - 需要自定义协议、实时消息推送、广播机制
- **微服务内部通信**
  - 使用 UDP/TCP 替代 HTTP，降低开销，提升吞吐
- **教学与实验**
  - 学习网络编程、事件驱动模型的一个简单起点

---

#### API 设计

`uno` 的对外 API 设计极为简洁：

```go
// 启动一个 TCP 或 UDP 服务，阻塞运行直到 Stop() 调用或出错
func Serve(ctx context.Context, hook core.ServerHook, addr string, opts ...Option) error 

// 连接一个 TCP 或 UDP 服务
func Dial(ctx context.Context, hook core.ConnHook, addr string, opts ...Option) (core.Conn, error)
```

**极简入口**
 整个框架只暴露两个核心方法：`Serve` 与 `Dial`。

- 服务端只需调用 `Serve` 即可启动监听，无需手动管理 `net.Listener`、`accept` 循环。
- 客户端只需调用 `Dial` 即可建立连接，无需关心 `net.Conn` 的复杂操作。
   这样开发者在入门时没有复杂的心智负担，只需关注业务逻辑。

**统一抽象**
 无论是 TCP 还是 UDP，API 保持一致：

- 都是 `Serve` / `Dial`
- 都是基于 `hook`（回调接口）处理事件
- 通过 `Option` 注入差异化配置
   开发者切换协议只需替换 `uno.WithNetwork("tcp"|"udp")`，业务层代码无需改动。

**事件驱动**
 框架没有强行要求继承复杂的基类，而是通过实现 `ServerHook` 或 `ConnHook` 接口获得回调。
 例如：

```
type EchoServer struct { uno.ServerEvent }
func (e *EchoServer) OnMessage(c core.Conn, msg any) { ... }
```

这种设计既清晰（只重写需要的回调），又保证了灵活性。

**上下文管理**
 `Serve` 和 `Dial` 都接收 `context.Context`，这让调用方能够：

- 控制服务/连接生命周期
- 支持超时与取消
- 更容易集成到大型系统中

**Option 模式**
 配置采用 `Option` 设计，而不是传入庞大的配置结构体，原因有：

- 保持调用简洁：`uno.Serve(ctx, hook, ":8080", uno.WithNetwork("udp"))`
- 按需配置：只设置自己关心的部分
- 易于扩展：未来新增选项时不破坏现有 API

----

#### 数据流向

在 `uno` 中，网络数据的处理被拆分为一系列有序的组件，每个组件只关注自己的一小部分职责。这样可以让整体结构清晰、可扩展，同时也方便用户按需替换某一环节的实现。

**消息接收流程**

```
Socket Read → Framer → Decoder → Handler Chain → OnMessage
```

1. **Socket Read**
   - 从底层 `net.Conn` 或 UDP 套接字中读取原始字节流。
2. **Framer**（拆帧器）
   - 解决 TCP 粘包、半包等问题。
   - 将原始字节流切分成完整的消息帧。
   - 例如：`LineFramer` 按换行符切分，`LengthFieldFramer` 按长度字段切分。
3. **Decoder**（解码器）
   - 将字节帧转化为业务层对象。
   - 例如：把 `[]byte` 解析成 JSON、Protobuf 或自定义结构体。
4. **Handler Chain**（中间件链）
   - 可选的业务前置处理流程。
   - 常见功能：日志记录、限流、鉴权、路由分发等。
   - 以链式调用的方式依次处理消息，类似于 HTTP 框架的中间件。
5. **OnMessage**（消息回调）
   - 消息流经中间件链且未被终止，则消息最终触发该回调。
   - 它是默认的最终处理点，而不是强制必达点。

---

**消息发送流程**

```
Send → Encoder → Socket Write
```

1. **Send**
   - 开发者调用 `Conn.Send(msg)` 发送一条消息。
2. **Encoder**（编码器）
   - 将业务对象编码成字节数组。
   - 与 Decoder 相对应，常见实现：JSON 编码、二进制协议编码等。
3. **Socket Write**
   - 将字节数组写入底层连接，由操作系统发送到远端。

---

#### 线程模型

`uno` 在设计上采用了 **固定职责 + 协程池调度** 的线程模型。
 这种设计既保证了网络 IO 的并发处理能力，又避免了 goroutine 无限制膨胀。

**Server**

- **Accept 协程**
  - 每个 `Server` 启动时，会开启一个 goroutine 负责 `accept` 新连接。
  - 连接建立后立即交给 `Conn` 管理。

**Conn**

每个连接独占 **三个核心 goroutine**：

1. **Read 协程**
   - 持续从底层 Socket 读取数据。
   - 读出的字节流会交给 `Framer → Decoder → HandlerChain → OnMessage `  处理 （这一过程提交至Pool执行）。
2. **Write 协程**
   - 负责从发送队列中取出消息，并写入 Socket。
   - 保证同一连接的写操作有序，避免并发写冲突。
3. **Control 协程**
   - 管理连接生命周期，包括超时检测、心跳、定时任务、关闭信号等。
   - 作为连接的“调度中枢”。

**协程池**

除此之外，`uno` 还内置了一个 **通用协程池**，用于承载业务逻辑与索引事件：

```
type Pool interface {
    Submit(task func()) bool
}
```

- 框架在需要执行 **用户回调（OnMessage、OnConnect 等）** 或 **中间件逻辑** 时，会将任务提交到 `Pool`。
- `Pool` 的实现可以自由替换（例如：有界队列、阻塞队列、超时销毁等）。
- 这样保证业务逻辑不会与核心 IO 读写抢占同一个 goroutine，提升系统整体的 **稳定性** 和 **吞吐能力**。

---

### End

