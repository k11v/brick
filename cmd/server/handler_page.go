package main

import "net/http"

type ExecutePageParams struct {
	mainParams *ExecuteMainParams
}

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	page, err := h.execute("build.html.tmpl", &ExecutePageParams{
		mainParams: &ExecuteMainParams{},
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}
