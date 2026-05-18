package subscription

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/sakhtar/xray-stack-zeroone/internal/links"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/usage"
)

// Deps lets the handler reach config + usage state without depending on
// the api package (avoids an import cycle — api imports us).
type Deps struct {
	Config        func() stack.Config
	LoadUsage     func() (usage.UserState, error)
	PortalBaseURL func(r *http.Request) string // e.g. "https://skhatar.runflare.run"
}

//go:embed portal.html
var portalHTML string

//go:embed qrcode.min.js
var qrcodeJS string

var portalTmpl = template.Must(template.New("portal").Parse(portalHTML))

// HandleSubscription is the GET /sub/{token} handler. It looks the user
// up by token, renders their links, and returns them in the format the
// client negotiated for via Accept / UA / ?format=.
func HandleSubscription(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/sub/")
		token = strings.Trim(token, "/")
		cfg := d.Config()
		user, ok := cfg.UserBySubToken(token)
		if !ok {
			http.NotFound(w, r)
			return
		}

		all := links.VLESS(cfg, user)
		format := NegotiateFormat(r)
		body, ct := Encode(format, all)

		info := buildUserInfo(d, user)
		base := d.PortalBaseURL(r)
		WriteResponseHeaders(w.Header(), info, user.Email, base+"/me/"+token, 24)
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", `attachment; filename="`+user.Email+`-`+format.String()+`.txt"`)
		_, _ = w.Write(body)
	}
}

// HandlePortal serves the user-facing HTML page at /me/{token}.
func HandlePortal(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/me/")
		token = strings.Trim(token, "/")
		cfg := d.Config()
		user, ok := cfg.UserBySubToken(token)
		if !ok {
			http.NotFound(w, r)
			return
		}
		all := links.VLESS(cfg, user)
		info := buildUserInfo(d, user)
		data := portalData{
			Email:        user.Email,
			Token:        token,
			Enabled:      user.Enabled,
			BannedUntil:  user.BannedUntil,
			Quota:        user.QuotaBytes,
			DownloadMbps: user.DownloadMbps,
			UploadMbps:   user.UploadMbps,
			Used:         info.UploadBytes + info.DownloadBytes,
			Upload:       info.UploadBytes,
			Download:     info.DownloadBytes,
			BaseURL:      d.PortalBaseURL(r),
			Links:        all,
		}
		// Pre-serialize link list as JSON so the embedded JS can iterate
		// without HTML-escaping headaches in the template.
		if b, err := json.Marshal(all); err == nil {
			data.LinksJSON = template.JS(b)
		}
		if b, err := json.Marshal(data); err == nil {
			data.AllJSON = template.JS(b)
		}
		data.QRCodeJS = template.JS(qrcodeJS)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = portalTmpl.Execute(w, data)
	}
}

type portalData struct {
	Email        string
	Token        string
	Enabled      bool
	BannedUntil  int64
	Quota        int64
	DownloadMbps int
	UploadMbps   int
	Used         int64
	Upload       int64
	Download     int64
	BaseURL      string
	Links        []links.Link
	LinksJSON    template.JS
	AllJSON      template.JS
	QRCodeJS     template.JS
}

func buildUserInfo(d Deps, user stack.User) UserInfo {
	info := UserInfo{TotalBytes: user.QuotaBytes}
	if user.BannedUntil > 0 {
		info.ExpireUnix = user.BannedUntil
	}
	if d.LoadUsage != nil {
		if st, err := d.LoadUsage(); err == nil {
			if p, ok := st.Totals[user.Email]; ok {
				info.UploadBytes = p.Uplink
				info.DownloadBytes = p.Downlink
			}
		}
	}
	return info
}
