package main

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/build"
)

func (h *Handler) CancelButtonClickToMain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Cookie access_token.
	const cookieAccessToken = "access_token"
	accessTokenCookie, err := r.Cookie(cookieAccessToken)
	if err != nil {
		h.serveError(w, r, fmt.Errorf("%s cookie: %w", cookieAccessToken, err))
		return
	}
	accessToken, err := parseAndValidateTokenFromCookie(ctx, h.db, h.jwtVerificationKey, accessTokenCookie)
	if err != nil {
		h.serveError(w, r, fmt.Errorf("%s cookie: %w", cookieAccessToken, err))
		return
	}
	userID := accessToken.UserID

	// Form value id.
	const formValueID = "id"
	id, err := uuid.Parse(r.FormValue(formValueID))
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s form value: %w", formValueID, err))
		return
	}

	canceler := build.NewCanceler(h.db)
	b, err := canceler.Cancel(ctx, &build.CancelerCancelParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	getter := build.NewGetter(h.db, h.s3)
	files, err := getter.GetFiles(ctx, &build.GetterGetParams{
		ID:     b.ID,
		UserID: userID,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	executeFilesParams, err := ExecuteFilesParamsFromFiles(files)
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	comp, err := h.execute("build_main", &ExecuteMainParams{
		Build: b,
		Files: executeFilesParams,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(comp)
}
