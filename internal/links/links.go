package links

import (
	"fmt"
	"net/url"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func VLESS(cfg stack.Config, user stack.User) []Link {
	if !user.Enabled {
		return nil
	}
	host := cfg.Server.PublicIP
	out := []Link{
		vlessLink(user.Email+"-ws", user.UUID, host, cfg.Xray.Inbounds.VLESSWSPort, "ws", "/vless"),
	}
	if cfg.Xray.Inbounds.VLESSXHTTPPort > 0 {
		out = append(out, vlessLink(user.Email+"-xhttp", user.UUID, host, 80, "xhttp", "/xhttp"))
	}
	if user.BandwidthPort > 0 && (user.DownloadMbps > 0 || user.UploadMbps > 0) {
		out = append(out, vlessLink(user.Email+"-limited", user.UUID, host, user.BandwidthPort, "ws", "/limited/"+user.Email))
	}
	return out
}

func SOCKS(cfg stack.Config, socks stack.SOCKSInbound) []Link {
	host := cfg.Server.PublicIP
	auth := url.QueryEscape(socks.Username) + ":" + url.QueryEscape(socks.Password)
	name := url.QueryEscape(socks.Username + "-socks")
	return []Link{
		{Name: "SOCKS5 URI", URL: fmt.Sprintf("socks5://%s@%s:%d#%s", auth, host, socks.Port, name)},
		{Name: "Host", URL: fmt.Sprintf("%s:%d", host, socks.Port)},
		{Name: "Username", URL: socks.Username},
		{Name: "Password", URL: socks.Password},
	}
}

func vlessLink(name, uuid, host string, port int, network, path string) Link {
	values := url.Values{}
	values.Set("encryption", "none")
	values.Set("type", network)
	values.Set("security", "none")
	values.Set("path", path)
	return Link{Name: name, URL: fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, host, port, values.Encode(), url.QueryEscape(name))}
}
