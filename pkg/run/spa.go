/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"bytes"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"go.arpabet.com/consensusdb/pkg/webui"
)

/*
Serving for the two embedded web apps (built by `make webui` from webapp/ into
pkg/webui, one build with two HTML entries and shared assets):

  - the read-only dashboard  (dashboard.html) at /dashboard (/ redirects here)
  - the admin console         (console.html)   at /console
  - their shared built assets                  at /assets

Both are single-page apps; their assets are content-hashed and live under
/assets, so each page mount just serves its one HTML entry (with a placeholder
when the binary was built without the embedded apps).
*/

func embeddedModTime() time.Time {
	if info, err := webui.AssetInfo("dashboard.html"); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// serveEmbedded writes an embedded asset with a content-type and cache header, or
// 404 if it is not present.
func serveEmbedded(w http.ResponseWriter, r *http.Request, name string, modTime time.Time) {
	data, err := webui.Asset(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ctype)
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // content-hashed
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, modTime, bytes.NewReader(data))
}

// PageHandler serves one embedded single-page app (a fixed HTML entry) at a mount.
type PageHandler struct {
	pattern string
	page    string
	modTime time.Time
	ok      bool
}

// NewPageHandler serves the embedded page at the given gorilla/mux pattern
// (e.g. "/" → index.html, "/console/{rest:.*}" → console.html).
func NewPageHandler(pattern, page string) *PageHandler {
	return &PageHandler{pattern: pattern, page: page}
}

func (t *PageHandler) BeanName() string { return "page-" + t.page }

func (t *PageHandler) PostConstruct() error {
	if info, err := webui.AssetInfo(t.page); err == nil {
		t.ok = true
		t.modTime = info.ModTime()
	}
	return nil
}

func (t *PageHandler) Pattern() string { return t.pattern }

func (t *PageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !t.ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder))
		return
	}
	serveEmbedded(w, r, t.page, t.modTime)
}

// AssetsHandler serves the shared built assets under /assets.
type AssetsHandler struct {
	modTime time.Time
}

func NewAssetsHandler() *AssetsHandler { return &AssetsHandler{} }

func (t *AssetsHandler) BeanName() string { return "assets-handler" }

func (t *AssetsHandler) PostConstruct() error { t.modTime = embeddedModTime(); return nil }

func (t *AssetsHandler) Pattern() string { return "/assets/{rest:.*}" }

func (t *AssetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/") // "assets/<file>"
	if !strings.HasPrefix(name, "assets/") {
		http.NotFound(w, r)
		return
	}
	serveEmbedded(w, r, name, t.modTime)
}

// Redirect issues a permanent redirect from a fixed path to a target — used to
// send / to the dashboard and the bare /dashboard and /console to their
// trailing-slash page mounts (which gorilla/mux only matches with the slash).
type Redirect struct {
	from, to string
}

func NewRedirect(from, to string) *Redirect { return &Redirect{from: from, to: to} }

func (t *Redirect) BeanName() string { return "redirect-" + t.from }

func (t *Redirect) Pattern() string { return t.from }

func (t *Redirect) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, t.to, http.StatusMovedPermanently)
}

const placeholder = `<!doctype html><html><head><meta charset="utf-8"><title>ConsensusDB</title></head>
<body style="font-family:system-ui;max-width:40rem;margin:4rem auto;line-height:1.5">
<h2>ConsensusDB web apps not embedded</h2>
<p>This binary was built without the embedded dashboard/console. Regenerate and rebuild:</p>
<pre style="background:#f4f4f4;padding:1rem;border-radius:6px">make webui   # npm run build + go-bindata → pkg/webui
go build</pre>
<p>The REST API is live at <code>/api/</code>.</p>
</body></html>`
