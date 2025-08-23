package conn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
	"uno/internal/conf"
	"uno/internal/hook"
)

type NETTransport struct {
	raw net.Conn
	cfg *conf.Config
}

func NewNETConn(ctx context.Context, raw net.Conn, cfg *conf.Config, hook hook.ConnHook) *Conn {
	// TCP 优化
	if t, ok := raw.(*net.TCPConn); ok {
		if cfg.KeepAlive {
			_ = t.SetKeepAlive(true)
			if cfg.KeepAlivePeriod > 0 {
				_ = t.SetKeepAlivePeriod(cfg.KeepAlivePeriod)
			}
		}
		if cfg.NoDelay {
			_ = t.SetNoDelay(true)
		}
	}
	t := &NETTransport{raw: raw, cfg: cfg}
	c := NewConn(ctx, t, cfg, hook)
	return c
}

func (nt *NETTransport) LocalAddr() net.Addr {
	return nt.raw.LocalAddr()
}

func (nt *NETTransport) RemoteAddr() net.Addr {
	return nt.raw.RemoteAddr()
}

func (nt *NETTransport) Write(c *Conn, buf []byte) error {
	if nt.raw == nil {
		return net.ErrClosed
	}

	// UDP MTU 检查
	if _, ok := nt.raw.(*net.UDPConn); ok && nt.cfg.MTU > 0 && len(buf) > nt.cfg.MTU {
		return fmt.Errorf("udp: payload exceeds MTU")
	}

	// 写超时设置
	timeout := nt.cfg.WriteTimeout
	if timeout <= 0 {
		timeout = WriteTimeout
	}

	_ = nt.raw.SetWriteDeadline(time.Now().Add(timeout))
	_, err := nt.raw.Write(buf)
	return err
}

func (nt *NETTransport) Start(c *Conn) {
	c.Wg.Add(1)
	go func() {
		defer c.Wg.Done()
		buf := make([]byte, c.Cfg.ReadBufferSize)

		for {
			select {
			case <-c.Context().Done():
				return
			default:
			}

			_ = nt.raw.SetReadDeadline(time.Now().Add(ReadTimeout))
			n, err := nt.raw.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				c.Recv(chunk)
			}

			if err != nil {
				// 本端关闭 / 对端 EOF → 触发会话关闭信号
				if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
					c.Cancel()
					return
				}

				// 读超时（deadline 到期）/ 临时错误（网络抖动）继续循环，等待下次读
				var ne net.Error
				if errors.As(err, &ne) && (ne.Timeout() || ne.Temporary()) {
					continue
				}

				// 不可恢复错误 → 触发 错误回调 和 会话关闭信号
				c.dispatchError(err)
				c.Cancel()
				return
			}
		}
	}()
}

func (nt *NETTransport) Stop(c *Conn) {
	if nt.raw != nil {
		_ = nt.raw.Close()
		nt.raw = nil
	}
}
