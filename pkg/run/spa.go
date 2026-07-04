/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

/*
SpaHandler serves the web admin console (the Vue + Vite app in webapp/) under
/console. It serves the built assets from webapp.dir (default ./webapp/dist) with
a single-page-app fallback: unknown non-asset paths return index.html so the
client-side router handles them. When the app has not been built, it serves a
short placeholder with build instructions, so the server binary always runs.

Production images build the SPA (npm ci && npm run build) and either bake
webapp/dist into the image or set webapp.dir to a mounted copy.
*/
type SpaHandler struct {
	Dir string `value:"webapp.dir,default=webapp/dist"`

	fs http.Handler
}

func NewSpaHandler() *SpaHandler { return &SpaHandler{} }

func (t *SpaHandler) BeanName() string { return "spa-handler" }

func (t *SpaHandler) PostConstruct() error {
	if st, err := os.Stat(filepath.Join(t.Dir, "index.html")); err == nil && !st.IsDir() {
		t.fs = http.FileServer(http.Dir(t.Dir))
	}
	return nil
}

func (t *SpaHandler) Pattern() string { return "/console/{rest:.*}" }

func (t *SpaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.fs == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder))
		return
	}
	// Strip the /console prefix, then fall back to index.html for SPA routes.
	rel := strings.TrimPrefix(r.URL.Path, "/console")
	if rel == "" || rel == "/" {
		rel = "/index.html"
	}
	if _, err := os.Stat(filepath.Join(t.Dir, filepath.Clean(rel))); os.IsNotExist(err) && !strings.Contains(rel, ".") {
		rel = "/index.html"
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = rel
	t.fs.ServeHTTP(w, r2)
}

const placeholder = `<!doctype html><html><head><meta charset="utf-8"><title>ConsensusDB Console</title></head>
<body style="font-family:system-ui;max-width:40rem;margin:4rem auto;line-height:1.5">
<h2>ConsensusDB admin console</h2>
<p>The web console has not been built yet. From <code>webapp/</code>:</p>
<pre style="background:#f4f4f4;padding:1rem;border-radius:6px">npm ci
npm run build</pre>
<p>Then reload — the built app is served from <code>webapp/dist</code>. The REST
API it calls is live at <code>/api/</code>.</p>
</body></html>`
