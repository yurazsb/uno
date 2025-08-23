package encoder

import (
	"encoding/json"
	"fmt"
	"github.com/yurazsb/uno/internal/boot"
)

// Encoder 编码器
type Encoder func(c boot.Conn, msg any) (buf []byte, err error)

// GenericEncoder 返回一个通用编码器 (Encoder)。
//
// 功能说明：
//   - 支持常用基础类型编码为字节切片，包括:
//     []byte, string, int/uint 系列, float32/64, bool
//   - 对其他类型 (struct、map、slice 等) 自动进行 JSON 序列化
//
// 使用场景：
//   - 适用于简单协议或通用消息编码
//   - 当消息类型多样且不固定时，可以直接使用 GenericEncoder
var GenericEncoder = func() Encoder {
	return func(c boot.Conn, msg any) (buf []byte, err error) {
		switch v := msg.(type) {
		case []byte:
			return v, nil
		case string:
			return []byte(v), nil
		case int, int8, int16, int32, int64:
			return []byte(fmt.Sprintf("%d", v)), nil
		case uint, uint8, uint16, uint32, uint64:
			return []byte(fmt.Sprintf("%d", v)), nil
		case float32, float64:
			return []byte(fmt.Sprintf("%f", v)), nil
		case bool:
			if v {
				return []byte("true"), nil
			}
			return []byte("false"), nil
		default:
			// 尝试 JSON 编码
			buf, err = json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("DefaultEncoder: unsupported type %T, json marshal failed: %w", msg, err)
			}
			return buf, nil
		}
	}
}
