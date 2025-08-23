package uuid

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"time"
)

// base62 字符集（更紧凑，适合 URL / ID 使用）
const base62Charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// EncodeBase62 将整数编码为 base62 字符串
func EncodeBase62(num uint64) string {
	if num == 0 {
		return "0"
	}
	sb := strings.Builder{}
	for num > 0 {
		rem := num % 62
		sb.WriteByte(base62Charset[rem])
		num /= 62
	}
	// 因为是从低位写起，需要反转
	runes := []rune(sb.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// NanoID 生成一个 base62 编码的唯一 ID，默认长度约 12~20 位（取决于时间和随机量）
// length 参数可截断结果（建议 >= 8）
func NanoID(length int) string {
	now := uint64(time.Now().UnixNano())

	// 添加一些随机位（4字节）
	rb := make([]byte, 4)
	_, _ = rand.Read(rb)
	randNum := binary.BigEndian.Uint32(rb)

	combined := (now << 32) | uint64(randNum)
	id := EncodeBase62(combined)

	if length > 0 && length < len(id) {
		return id[:length]
	}
	return id
}
