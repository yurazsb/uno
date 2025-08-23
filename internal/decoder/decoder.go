package decoder

import (
	"github.com/yurazsb/uno/internal/boot"
	"strings"
)

// Decoder 解码器
type Decoder func(c boot.Conn, buf []byte) (msg any, err error)

// RawDecoder 返回一个原始二进制解码器 。
// 它直接返回接收到的字节切片，不做任何解析或转换。
//
// 使用说明:
//   - 常用于不需要解码的二进制数据传输场景
//   - 可与任何 Framer 搭配使用
var RawDecoder = func() Decoder {
	return func(c boot.Conn, buf []byte) (any, error) {
		return buf, nil
	}
}

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
var StringDecoder = func(trim bool) Decoder {
	return func(c boot.Conn, buf []byte) (any, error) {
		s := string(buf)
		if trim {
			s = strings.TrimSpace(s)
		}
		return s, nil
	}
}
