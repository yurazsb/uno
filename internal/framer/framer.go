package framer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/yurazsb/uno/internal/boot"
)

// Framer 拆帧 解码器接口
type Framer func(c boot.Conn, buf []byte) (frames [][]byte, remaining []byte, err error)

// RawFramer 返回一个“直通式”的帧解码器 (Framer)。
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
// 返回值：
//   - frames:    仅包含一条帧，即当前 buf 全部数据。
//   - remaining: 始终为 nil（因为没有保留半包的逻辑）。
//   - err:       始终为 nil。
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
var RawFramer = func() Framer {
	return func(c boot.Conn, buf []byte) (frames [][]byte, remaining []byte, err error) {
		frames = append([][]byte{}, buf)
		return
	}
}

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
//	输入字节流: "HELLO\nWORLD\nTEST"
//	解码结果: ["HELLO", "WORLD"]
//	剩余: "TEST"
//
// 等待后续输入拼接 "TEST\n" 后，得到完整帧 "TEST"。
var LineFramer = func() Framer {
	return DelimiterFramer([]byte("\n"))
}

// DelimiterFramer 返回一个基于“分隔符”的帧解码器 (Framer)。
//
// 该解码器通过指定的分隔符 (delim) 将字节流拆分为多帧。
// 每当遇到分隔符时，之前的数据段会作为一帧返回，分隔符本身会被丢弃。
// 剩余未遇到分隔符的数据将存入 remaining，以便下次拼接继续解析。
//
// 参数说明：
//   - delim: 消息之间的分隔符。例如 "\n"、"\r\n"、"\0"。
//
// 返回值：
//   - frames:     已解析出的完整消息帧（不包含分隔符）。
//   - remaining:  未遇到分隔符的剩余字节。
//   - err:        始终为 nil（本实现中不会返回错误）。
//
// 使用场景：
//   - 文本协议（Telnet、Redis RESP、SMTP、POP3 等）。
//   - 行协议、命令式协议、日志传输。
//   - 简单消息协议，以换行或其他字符作为结束符。
//
// 协议示例：
//
//	输入字节流: "PING\nPONG\nPARTIAL"
//	分隔符: "\n"
//	解码结果: ["PING", "PONG"]
//	剩余: "PARTIAL"  (等待下次拼接)
var DelimiterFramer = func(delim []byte) Framer {
	return func(c boot.Conn, buf []byte) (frames [][]byte, remaining []byte, err error) {
		for {
			idx := bytes.Index(buf, delim)
			if idx == -1 {
				break
			}
			frames = append(frames, buf[:idx])
			buf = buf[idx+len(delim):]
		}
		remaining = buf
		return
	}
}

// FixedLengthFramer 返回一个基于“固定长度”的帧解码器 (Framer)。
//
// 该解码器假设每条消息的长度固定为 length 字节。
// 在解析过程中，会不断从 buf 中切割出定长帧，直到剩余数据不足一帧为止。
// 剩余未满一帧的数据将存入 remaining，以便下次拼接。
//
// 参数说明：
//   - length: 每个消息帧的固定字节数（必须大于 0）。
//
// 返回值：
//   - frames:     已解析出的完整定长消息帧切片。
//   - remaining:  未能组成完整帧的剩余字节。
//   - err:        始终为 nil（本实现中不会返回错误）。
//
// 使用场景：
//   - 固定包长的二进制协议或嵌入式设备通信协议。
//   - 游戏服务器/IoT 设备中部分简化协议。
//   - 日志采集、传感器数据、简易传输格式。
//
// 协议示例：
//
//	输入字节流: "ABCDEFGHIJK"
//	配置: FixedLengthFramer(3)
//	解码过程:
//	  → "ABC"
//	  → "DEF"
//	  → "GHI"
//	剩余: "JK"
//	（等待后续拼接两个字节，组成下一条完整消息 "JKX"...）
var FixedLengthFramer = func(length int) Framer {
	return func(c boot.Conn, buf []byte) (frames [][]byte, remaining []byte, err error) {
		for len(buf) >= length {
			frames = append(frames, buf[:length])
			buf = buf[length:]
		}
		remaining = buf
		return
	}
}

// LengthFieldFramer 返回一个基于“长度字段”的帧解码器 (Framer)。
//
// 该解码器假设消息包格式为：
//
//	[header | length field | payload]
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
//     例如：某些协议长度字段仅表示数据体长度，若还需要包含头部其他部分，可以通过此值修正。
//   - initialBytesToStrip:   从最终帧中需要剥离的前置字节数。常用于跳过协议头，仅保留 payload。
//     例如：设置为 lengthFieldOffset+lengthFieldSize，可以直接返回数据体部分。
//   - order:                 长度字段的字节序（binary.BigEndian 或 binary.LittleEndian）。
//
// 返回值：
//   - frames:     已经解析出的完整帧切片。
//   - remaining:  剩余未能组成完整帧的字节数据。
//   - err:        协议不合法或长度字段错误时返回的错误。
//
// 使用场景：
//   - 自定义二进制协议：常见于 TCP 协议栈或 RPC 框架。
//   - Protobuf/Thrift 封装：通常前 4 个字节为消息长度。
//   - Netty LengthFieldBasedFrameDecoder 的 Go 实现。
//
// 协议示例 1：简单长度字段协议
//
//	格式: [4字节长度][payload]
//	配置: LengthFieldFramer(0, 4, 0, 4, binary.BigEndian)
//	示例: 00 00 00 05  48 65 6C 6C 6F
//	     |--len=5--|  H  e  l  l  o
//	解码结果: "Hello"
//
// 协议示例 2：带魔数与长度字段
//
//	格式: [2字节魔数][4字节长度][payload]
//	配置: LengthFieldFramer(2, 4, 0, 6, binary.BigEndian)
//	示例: 0xAB 0xCD  00 00 00 05  48 65 6C 6C 6F
//	       magic    |--len=5-|   payload
//	解码结果: "Hello"
//
// 协议示例 3：长度字段包含包头长度
//
//	格式: [4字节长度(含头)][payload]
//	配置: LengthFieldFramer(0, 4, -4, 4, binary.BigEndian)
//	示例: 00 00 00 09  01 02 03 04 05
//	     |---len=9 (4字节头+5字节体)--|
//	解码结果: 01 02 03 04 05
var LengthFieldFramer = func(lengthFieldOffset, lengthFieldSize, lengthAdjustment, initialBytesToStrip int, order binary.ByteOrder) Framer {
	return func(c boot.Conn, buf []byte) (frames [][]byte, remaining []byte, err error) {
		for {
			// 长度字段未完整
			if len(buf) < lengthFieldOffset+lengthFieldSize {
				break
			}
			// 取出长度字段
			var frameLen uint64
			fieldBytes := buf[lengthFieldOffset : lengthFieldOffset+lengthFieldSize]
			switch lengthFieldSize {
			case 1:
				frameLen = uint64(fieldBytes[0])
			case 2:
				frameLen = uint64(order.Uint16(fieldBytes))
			case 4:
				frameLen = uint64(order.Uint32(fieldBytes))
			case 8:
				frameLen = order.Uint64(fieldBytes)
			default:
				err = fmt.Errorf("unsupported lengthFieldSize=%d", lengthFieldSize)
				return
			}
			frameLen += uint64(lengthAdjustment)

			// 总包长 = lengthFieldOffset + lengthFieldSize + frameLen
			frameEnd := lengthFieldOffset + lengthFieldSize + int(frameLen)
			if len(buf) < frameEnd {
				break
			}

			// 截取完整帧
			frame := buf[initialBytesToStrip:frameEnd]
			frames = append(frames, frame)

			// 进入下一个
			buf = buf[frameEnd:]
		}
		remaining = buf
		return
	}
}
