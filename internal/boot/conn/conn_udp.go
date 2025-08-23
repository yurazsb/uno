package conn

import (
	"context"
	"fmt"
	"github.com/yurazsb/uno/internal/boot"
	"github.com/yurazsb/uno/internal/conf"
	"github.com/yurazsb/uno/internal/hook"
	"net"
	"sync"
	"time"
)

// ---- Key 优化（避免 addr.String() 带来的分配与 GC）----

type udpKey struct {
	ip   [16]byte // 支持 IPv4/IPv6（IPv4 映射）
	port uint16
}

func ucKey(a *net.UDPAddr) udpKey {
	var k udpKey
	if ip4 := a.IP.To4(); ip4 != nil {
		k.ip[10], k.ip[11] = 0xff, 0xff
		copy(k.ip[12:], ip4)
	} else {
		copy(k.ip[:], a.IP.To16()) // IPv6
	}
	k.port = uint16(a.Port)
	return k
}

type UDPSession struct {
	raw     *net.UDPConn
	cfg     *conf.Config
	hook    hook.ConnHook
	log     boot.Logger
	connMap sync.Map // udpKey -> *Conn
}

func NewUDPSession(raw *net.UDPConn, cfg *conf.Config, hook hook.ConnHook) *UDPSession {
	return &UDPSession{raw: raw, cfg: cfg, hook: hook, log: cfg.Logger}
}

func (us *UDPSession) Delivery(ctx context.Context, wg *sync.WaitGroup, remote *net.UDPAddr, buf []byte) {
	key := ucKey(remote)
	val, ok := us.connMap.Load(key)

	if !ok {
		// 为该 remote 创建一个伪连接
		ut := newUDPChildTransport(us, us.raw, remote)
		uc := NewConn(ctx, ut, us.cfg, us.hook)
		actual, loaded := us.connMap.LoadOrStore(key, uc)
		if loaded {
			val = actual
		} else {
			val = uc
			uc.Start(wg)
		}
	}

	uc := val.(*Conn)
	ut := uc.T.(*UDPTransport)
	select {
	case ut.recvCh <- buf:
		return
	default:
	}
}

func (us *UDPSession) Reaper(idle time.Duration) {
	now := time.Now()
	us.connMap.Range(func(k, v any) bool {
		if uc, ok := v.(*Conn); ok {
			if since := now.Sub(uc.LastActive()); since > idle {
				us.connMap.Delete(k)
				uc.Close()
			}
		}
		return true
	})
}

func (us *UDPSession) Clear(remotes ...*net.UDPAddr) {
	if len(remotes) == 0 {
		us.connMap.Range(func(key, val interface{}) bool {
			uc := val.(*Conn)
			uc.Close()
			return true
		})
		us.connMap.Clear()
		return
	}

	for _, remote := range remotes {
		key := ucKey(remote)
		val, loaded := us.connMap.LoadAndDelete(key)
		if loaded {
			uc := val.(*Conn)
			uc.Close()
		}
	}
}

type UDPTransport struct {
	session *UDPSession
	raw     *net.UDPConn
	remote  *net.UDPAddr
	recvCh  chan []byte
	cfg     *conf.Config
}

func newUDPChildTransport(us *UDPSession, raw *net.UDPConn, remote *net.UDPAddr) *UDPTransport {
	return &UDPTransport{
		session: us,
		raw:     raw,
		remote:  remote,
		recvCh:  make(chan []byte, 10_000),
		cfg:     us.cfg,
	}
}

func (ut *UDPTransport) LocalAddr() net.Addr {
	return ut.raw.LocalAddr()
}

func (ut *UDPTransport) RemoteAddr() net.Addr {
	return ut.remote
}

func (ut *UDPTransport) Write(c *Conn, buf []byte) error {
	if ut.raw == nil {
		return net.ErrClosed
	}

	// UDP MTU 检查
	if ut.cfg.MTU > 0 && len(buf) > ut.cfg.MTU {
		return fmt.Errorf("udp: payload exceeds MTU")
	}

	// 写超时设置
	timeout := ut.cfg.WriteTimeout
	if timeout <= 0 {
		timeout = WriteTimeout
	}

	_ = ut.raw.SetWriteDeadline(time.Now().Add(timeout))
	_, err := ut.raw.WriteToUDP(buf, ut.remote)
	return err
}

func (ut *UDPTransport) Start(c *Conn) {
	c.Wg.Add(1)
	go func() {
		defer c.Wg.Done()
		for {
			select {
			case <-c.Context().Done():
				return
			case buf, ok := <-ut.recvCh:
				if !ok {
					return
				}
				c.Recv(buf) // Delivery 已拷贝过，无需二次 copy
			}
		}
	}()
}

func (ut *UDPTransport) Stop(c *Conn) {
	// 从 session 的 map 删除（注意 key 类型一致）
	if ut.session != nil && ut.remote != nil {
		key := ucKey(ut.remote)
		ut.session.connMap.Delete(key)
	}
	close(ut.recvCh)
}
