// Package shared 提供跨层共享的基础设施
// safehttp.go — 带 SSRF 防护的 HTTP 客户端
package shared

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// isBlockedIP 判断目标 IP 是否属于禁止访问的内网/保留地址段（SSRF 防护）
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// 云元数据地址（AWS/GCP/阿里云等）169.254.169.254 已被 LinkLocal 覆盖，这里再显式拦截
	if ip4 := ip.To4(); ip4 != nil {
		// 额外拦截内部约定网段：9.* / 11.* / 21.* / 30.*（按安全基线要求）
		switch ip4[0] {
		case 9, 11, 21, 30:
			return true
		}
		// 100.64.0.0/10 (CGNAT)
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

// safeControl 在连接建立前校验目标 IP，阻断指向内网/保留地址的连接（防 DNS rebinding）
func safeControl(network, address string, c syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("解析目标地址失败: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("无效的目标 IP: %s", host)
	}
	if isBlockedIP(ip) {
		return fmt.Errorf("拒绝访问内网/保留地址: %s", host)
	}
	return nil
}

// SafeHTTPClient 返回带 SSRF 防护的 HTTP 客户端：
//   - 在连接层校验解析后的真实 IP，拒绝环回/私有/链路本地/保留网段
//   - 禁止自动跟随重定向（防止重定向到内网）
func SafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   safeControl,
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ValidateOutboundURL 在发起请求前校验 URL 主机解析出的所有 IP 是否安全
// 用于无法替换 http.Client 的场景；返回 nil 表示安全
func ValidateOutboundURL(ctx context.Context, host string) error {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("解析主机失败: %w", err)
	}
	for _, ipAddr := range ips {
		if isBlockedIP(ipAddr.IP) {
			return fmt.Errorf("拒绝访问内网/保留地址: %s", ipAddr.IP.String())
		}
	}
	return nil
}
