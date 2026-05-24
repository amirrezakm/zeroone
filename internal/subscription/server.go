package subscription

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"mime"
	"net/http"
	"strings"

	"github.com/amirrezakm/zeroone/internal/links"
	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/usage"
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

// qrLibMarker is the placeholder in portal.html where the vendored QR-code
// library is spliced in at startup.
const qrLibMarker = "<!-- qrcode-lib (injected at startup) -->"

// portalTmpl inlines the QR-code library into the page once, at startup.
// Keeping the library out of portal.html itself (the source only carries the
// marker comment above) means the .html file holds no inline JavaScript written
// as a Go-template action, so static analysers can parse the page's <script>
// blocks cleanly. qrcodeJS is a trusted, vendored asset that contains no
// template actions and no "</script" sequence, so this splice is safe.
var portalTmpl = template.Must(template.New("portal").Parse(
	strings.Replace(portalHTML, qrLibMarker, "<script>"+qrcodeJS+"</script>", 1),
))

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
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Build the attachment filename from a sanitised email token so a
		// crafted address can't inject CR/LF or extra header parameters;
		// mime.FormatMediaType handles any remaining quoting.
		filename := safeAttachmentName(user.Email) + "-" + format.String() + ".txt"
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
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
		// Serialize the portal data for the embedded
		// <script type="application/json"> block, which the page reads via
		// JSON.parse(el.textContent). encoding/json HTML-escapes the angle
		// brackets and ampersand to their \u00XX forms by default, so the
		// payload can never break out of the script element; template.JS then
		// emits that already-safe JSON verbatim. The value is parsed as data,
		// never eval'd, so this is XSS-safe.
		if b, err := json.Marshal(data); err == nil {
			data.AllJSON = template.JS(b)
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		// The portal is fully self-contained: inline <style>, inline <script>
		// (incl. the vendored QR lib), a JSON data island, and a canvas QR.
		// It fetches nothing and embeds no external origins.
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
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
	AllJSON      template.JS
}

// safeAttachmentName turns an arbitrary user label (typically an email
// address) into a filename token safe to embed in a Content-Disposition
// header. It keeps only [A-Za-z0-9._-]; every other byte — including CR, LF,
// quotes, semicolons, and path separators — becomes "_", which blocks header
// injection and path tricks. The result is trimmed of leading/trailing
// "._-", capped at 80 bytes, and falls back to "subscription" when nothing
// usable remains.
func safeAttachmentName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < 80; i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '.', c == '_', c == '-':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "subscription"
	}
	return out
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
