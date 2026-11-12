package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/web"
)

// ifaceInfo is one network interface on this computer, as plain data.
type ifaceInfo struct {
	Name  string
	Up    bool
	Addrs []string // "address/prefix", like 192.168.1.20/24
}

// systemInterfaces reads the interfaces from the system.
func systemInterfaces() []ifaceInfo {
	var out []ifaceInfo
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, i := range ifs {
		info := ifaceInfo{Name: i.Name, Up: i.Flags&net.FlagUp != 0 && i.Flags&net.FlagLoopback == 0}
		if addrs, err := i.Addrs(); err == nil {
			for _, a := range addrs {
				info.Addrs = append(info.Addrs, a.String())
			}
		}
		out = append(out, info)
	}
	return out
}

var vpnLike = regexp.MustCompile(`(?i)vpn|tap-|wireguard|wintun|tunnel|tailscale|zerotier|openvpn|lightway|^utun|^tun\d|^wg\d|^ppp`)

// localNet returns the address and network when the address is a private IPv4 one.
func localNet(ip string, prefix int) (net.IP, *net.IPNet, bool) {
	a := net.ParseIP(ip).To4()
	if a == nil || !a.IsPrivate() || prefix < 8 || prefix > 30 {
		return nil, nil, false
	}
	mask := net.CIDRMask(prefix, 32)
	return a, &net.IPNet{IP: a.Mask(mask), Mask: mask}, true
}

// findAddress picks the address to give a phone: the adapter carrying the
// default route, unless that is a VPN, then the first real network that is up.
func findAddress(ad collect.AdapterInfo, ifaces []ifaceInfo) (net.IP, *net.IPNet, bool) {
	if !vpnLike.MatchString(ad.Description) && !vpnLike.MatchString(ad.Name) {
		if ip, n, ok := localNet(ad.IP, ad.PrefixLength); ok {
			return ip, n, true
		}
	}
	for _, i := range ifaces {
		if !i.Up || vpnLike.MatchString(i.Name) {
			continue
		}
		for _, a := range i.Addrs {
			ip, n, err := net.ParseCIDR(a)
			if err != nil {
				continue
			}
			ones, _ := n.Mask.Size()
			if p, nn, ok := localNet(ip.String(), ones); ok {
				return p, nn, true
			}
		}
	}
	return nil, nil, false
}

// ShareOptions are the knobs for the share command. Zero values are the real ones.
type ShareOptions struct {
	Port       int
	Wait       time.Duration
	LightTerm  bool // draw the code for a light screen
	Serve      func(ctx context.Context, addr string, allow []string, run func() ([]byte, error), onRemote func(string)) error
	ServeTLS   func(ctx context.Context, addr string, allow []string, run func() ([]byte, error), onRemote func(string), cert tls.Certificate) error
	Interfaces []ifaceInfo
	CertDir    string // where the certificate is kept; empty means plain http
	Plain      bool   // the person asked for plain http
}

const (
	shareNoAddress = "I could not find this computer's network address. Check the cable or the Wi-Fi, then try again."
	shareHow       = "Open this on your phone. The phone must be on the same Wi-Fi or network as this computer."
	shareScan      = "Or point the phone camera at this code:"
	shareWaiting   = "Waiting for your phone to open the page. Press Ctrl+C when you are done."
	shareReached   = "Your phone reached the page. It works."
	shareNoPhone   = "No phone has opened the page yet. Try these:"
	shareAdviceWin = "If Windows asks to let Network Doctor in, press Allow. Pick Private networks."
	shareAdviceNet = "Make sure the phone is on the same Wi-Fi as this computer. A guest network often keeps devices apart."
	shareAdviceVPN = "A VPN is on in this computer. Turn it off for a moment, then try again."
	shareAdviceOff = "Turn off a VPN on the phone too, then scan the code again."
	shareWarn      = "The page is private. Your phone will say it is not secure, because the lock was made on this computer and the phone has not seen it before."
	shareWarnPress = "That is expected. Press Advanced, then Continue. It asks only once."
	shareNoCert    = "I could not make the lock for a private page, so I am using the plain address."
)

// shareTexts lists every sentence the command can print, for the reading test.
func shareTexts() []string {
	return []string{shareNoAddress, shareHow, shareScan, shareWaiting, shareReached, shareNoPhone, shareAdviceWin, shareAdviceNet, shareAdviceVPN, shareAdviceOff, shareWarn, shareWarnPress, shareNoCert}
}

// Share starts the page for phones on this network and shows how to open it.
// It never changes a firewall rule or any other setting.
func Share(ctx context.Context, src collect.Source, opt ShareOptions, copt Options, out io.Writer) int {
	if opt.Port == 0 {
		opt.Port = 8765
	}
	if opt.Wait == 0 {
		opt.Wait = time.Minute
	}
	if opt.Serve == nil {
		opt.Serve = web.ServeWith
	}
	if opt.Interfaces == nil {
		opt.Interfaces = systemInterfaces()
	}
	ad, _ := src.Adapter(ctx)
	ip, network, ok := findAddress(ad, opt.Interfaces)
	if !ok {
		fmt.Fprintln(out, shareNoAddress)
		return 2
	}
	scheme := "http"
	var cert tls.Certificate
	if opt.CertDir != "" && !opt.Plain {
		c, err := web.EnsureCert(opt.CertDir, []net.IP{ip}, time.Now())
		if err != nil {
			fmt.Fprintln(out, shareNoCert)
		} else {
			cert, scheme = c, "https"
		}
	}
	url := fmt.Sprintf("%s://%s:%d/", scheme, ip, opt.Port)
	addr := fmt.Sprintf("%s:%d", ip, opt.Port)
	fmt.Fprintln(out, shareHow)
	fmt.Fprintln(out, "  "+url)
	if scheme == "https" {
		fmt.Fprintln(out, shareWarn)
		fmt.Fprintln(out, shareWarnPress)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, shareScan)
	for _, l := range qrDraw(url, opt.LightTerm) {
		fmt.Fprintln(out, l)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, shareWaiting)

	var once sync.Once
	phone := make(chan string, 1)
	onRemote := func(from string) { once.Do(func() { phone <- from }) }

	sctx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() {
		run := func() ([]byte, error) { return json.Marshal(Check(sctx, src, copt)) }
		allow := []string{network.String()}
		if scheme == "https" {
			serveTLS := opt.ServeTLS
			if serveTLS == nil {
				serveTLS = web.ServeTLSWith
			}
			done <- serveTLS(sctx, addr, allow, run, onRemote, cert)
			return
		}
		done <- opt.Serve(sctx, addr, allow, run, onRemote)
	}()

	timer := time.NewTimer(opt.Wait)
	defer timer.Stop()
	advised := false
	for {
		select {
		case from := <-phone:
			fmt.Fprintf(out, "%s (%s)\n", shareReached, from)
			timer.Stop()
			advised = true
		case <-timer.C:
			if !advised {
				advised = true
				fmt.Fprintln(out, shareNoPhone)
				fmt.Fprintln(out, " -", shareAdviceWin)
				fmt.Fprintln(out, " -", shareAdviceNet)
				if st, err := src.Settings(ctx); err == nil && st.VPNActive {
					fmt.Fprintln(out, " -", shareAdviceVPN)
				}
				fmt.Fprintln(out, " -", shareAdviceOff)
			}
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				fmt.Fprintf(out, "I could not start the page on %s. Is it already open in another window?\n", addr)
				return 2
			}
			return 0
		case <-ctx.Done():
			<-done
			return 0
		}
	}
}

// qrDraw draws the code with half blocks, two rows of squares per line, inside
// a clear border four squares wide. By default light squares are drawn as
// blocks, so the code reads right on a dark screen; light=true is for a light
// screen, where dark squares are the blocks.
func qrDraw(text string, light bool) []string {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil
	}
	const border = 4
	size := c.Size + 2*border
	dark := func(x, y int) bool {
		x, y = x-border, y-border
		if x < 0 || y < 0 || x >= c.Size || y >= c.Size {
			return false
		}
		return c.Black(x, y)
	}
	block := func(top, bottom bool) rune {
		switch {
		case top && bottom:
			return '█'
		case top:
			return '▀'
		case bottom:
			return '▄'
		}
		return ' '
	}
	var lines []string
	for y := 0; y < size; y += 2 {
		var b strings.Builder
		for x := 0; x < size; x++ {
			top, bottom := dark(x, y), dark(x, y+1)
			if !light {
				top, bottom = !top, !bottom
			}
			b.WriteRune(block(top, bottom))
		}
		lines = append(lines, b.String())
	}
	return lines
}
