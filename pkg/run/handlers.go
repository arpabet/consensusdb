/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"net/http"
	"text/template"

	"go.arpabet.com/consensusdb/pkg/util"
)

/*
WelcomeHandler renders the welcome page at the root path. It is the catch-all
handler; more specific patterns (/metrics, /healthz) win.
*/
type WelcomeHandler struct {
	tpl *template.Template
}

func (t *WelcomeHandler) PostConstruct() error {
	t.tpl = util.MustAssetTemplate("templates/welcome.tmpl")
	return nil
}

func (t *WelcomeHandler) Pattern() string { return "/" }

func (t *WelcomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.tpl.Execute(w, r)
}
