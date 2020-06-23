/*
 *
 * Copyright 2020-present Arpabet Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
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
