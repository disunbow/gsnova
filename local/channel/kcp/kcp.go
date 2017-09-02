package kcp

import (
	"log"
	"net"
	"net/url"

	kcp "github.com/xtaci/kcp-go"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

// connectedUDPConn is a wrapper for net.UDPConn which converts WriteTo syscalls
// to Write syscalls that are 4 times faster on some OS'es. This should only be
// used for connections that were produced by a net.Dial* call.
type connectedUDPConn struct{ *net.UDPConn }

// WriteTo redirects all writes to the Write syscall, which is 4 times faster.
func (c *connectedUDPConn) WriteTo(b []byte, addr net.Addr) (int, error) { return c.Write(b) }

type KCPProxy struct {
	//proxy.BaseProxy
}

func (p *KCPProxy) Features() proxy.ProxyFeatureSet {
	return proxy.ProxyFeatureSet{
		AutoExpire: true,
		Pingable:   true,
	}
}

func (tc *KCPProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	hostport := rurl.Host
	tcpHost, tcpPort, _ := net.SplitHostPort(hostport)
	if net.ParseIP(tcpHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(tcpHost)
		if nil != err {
			return nil, err
		}
		hostport = net.JoinHostPort(iphost, tcpPort)
	}
	block, _ := kcp.NewNoneBlockCrypt(nil)

	udpaddr, err := net.ResolveUDPAddr("udp", hostport)
	if err != nil {
		return nil, err
	}
	udpconn, err := netx.DialUDP("udp", nil, udpaddr)
	if err != nil {
		return nil, err
	}
	kcpconn, err := kcp.NewConn(hostport, block, conf.KCP.DataShard, conf.KCP.ParityShard, &connectedUDPConn{udpconn.(*net.UDPConn)})
	//kcpconn, err := kcp.NewConn(hostport, block, conf.KCP.DataShard, conf.KCP.ParityShard, udpconn)
	//kcpconn, err := kcp.DialWithOptions(hostport, block, conf.KCP.DataShard, conf.KCP.ParityShard)
	if err != nil {
		return nil, err
	}
	kcpconn.SetStreamMode(true)
	kcpconn.SetWriteDelay(true)
	kcpconn.SetNoDelay(conf.KCP.NoDelay, conf.KCP.Interval, conf.KCP.Resend, conf.KCP.NoCongestion)
	kcpconn.SetWindowSize(conf.KCP.SndWnd, conf.KCP.RcvWnd)
	kcpconn.SetMtu(conf.KCP.MTU)
	kcpconn.SetACKNoDelay(conf.KCP.AckNodelay)

	if err := kcpconn.SetDSCP(conf.KCP.DSCP); err != nil {
		log.Println("SetDSCP:", err)
	}
	if err := kcpconn.SetReadBuffer(conf.KCP.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := kcpconn.SetWriteBuffer(conf.KCP.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}
	session, err := pmux.Client(kcpconn, proxy.InitialPMuxConfig(conf))
	if nil != err {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	return &mux.ProxyMuxSession{Session: session}, nil
}

func init() {
	proxy.RegisterProxyType("kcp", &KCPProxy{})
}
