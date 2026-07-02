/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package util

import (
  "net/http"
)

type gzipHandler struct {
  handler http.Handler
}

type gzipWriter struct {
  w http.ResponseWriter
}

func (t *gzipWriter) Header() http.Header {
  return t.w.Header()
}

func (t *gzipWriter) Write(b []byte) (int, error) {
  return t.w.Write(b)
}

func (t *gzipWriter) WriteHeader(statusCode int) {
  if statusCode == 200 {
    t.w.Header().Set("Content-Encoding", "gzip")
  }
  t.w.WriteHeader(statusCode)
}

func (t* gzipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  t.handler.ServeHTTP(&gzipWriter{w}, r)
}

func GzipHandler(handler http.Handler) http.Handler {
  return &gzipHandler {handler}
}
