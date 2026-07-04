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
SpaHandler serves the web admin console (the Vue + Vite app in webapp/) under
/console. The built assets are baked into the binary: `go-bindata` generates
pkg/webui/bindata.go from webapp/dist (see the Makefile `webui` target), so the
server is self-contained — no webapp/dist directory is needed at runtime.

It serves the embedded assets directly (avoiding http.FileServer's index.html
redirect) with a single-page-app fallback: an unknown, extension-less path
returns index.html so the client-side app boots. If the binary was built without
generating the assets, it serves a short placeholder so the server still runs.
*/
type SpaHandler struct {
	ok      bool
	modTime time.Time
}

func NewSpaHandler() *SpaHandler { return &SpaHandler{} }

func (t *SpaHandler) BeanName() string { return "spa-handler" }

func (t *SpaHandler) PostConstruct() error {
	// index.html present in the embedded set ⇒ the console was baked in.
	if info, err := webui.AssetInfo("index.html"); err == nil {
		t.ok = true
		t.modTime = info.ModTime()
	}
	return nil
}

func (t *SpaHandler) Pattern() string { return "/console/{rest:.*}" }

func (t *SpaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !t.ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder))
		return
	}

	// Strip the /console mount and normalize to an asset name (no leading slash).
	name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(r.URL.Path, "/console")), "/")
	if name == "" {
		name = "index.html"
	}

	data, err := webui.Asset(name)
	if err != nil {
		// SPA fallback: an unknown route without a file extension boots the app;
		// a missing real asset (has an extension) is a genuine 404.
		if strings.Contains(path.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		if data, err = webui.Asset(name); err != nil {
			http.NotFound(w, r)
			return
		}
	}

	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ctype)
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		// Vite emits content-hashed asset filenames, so they are immutable.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, name, t.modTime, bytes.NewReader(data))
}

const placeholder = `<!doctype html><html><head><meta charset="utf-8"><title>ConsensusDB Console</title></head>
<body style="font-family:system-ui;max-width:40rem;margin:4rem auto;line-height:1.5">
<h2>ConsensusDB admin console</h2>
<p>The console was not baked into this build. Regenerate the embedded assets and rebuild:</p>
<pre style="background:#f4f4f4;padding:1rem;border-radius:6px">make webui   # npm run build + go-bindata → pkg/webui
go build</pre>
<p>The REST API it calls is live at <code>/api/</code>.</p>
</body></html>`
