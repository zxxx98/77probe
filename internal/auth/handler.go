package auth

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"time"
)

const sessionMaxAge = 7 * 24 * 60 * 60

type Handler struct {
	service       *Service
	secureCookies bool
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service:       service,
		secureCookies: os.Getenv("TINYPROBE_SECURE_COOKIES") == "true",
	}
}

func (h *Handler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	required, err := h.service.SetupRequired(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		SetupRequired bool `json:"setupRequired"`
	}{SetupRequired: required})
}

func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	credentials, ok := decodeCredentials(w, r)
	if !ok {
		return
	}
	if err := h.service.CreateAdmin(r.Context(), credentials.Username, credentials.Password); err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err)
		case errors.Is(err, ErrSetupComplete):
			writeError(w, http.StatusConflict, err)
		default:
			writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
		}
		return
	}
	writeJSON(w, http.StatusCreated, Admin{ID: 1, Username: credentials.Username})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	credentials, ok := decodeCredentials(w, r)
	if !ok {
		return
	}
	token, expiresAt, err := h.service.Login(r.Context(), credentials.Username, credentials.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidLogin) {
			writeError(w, http.StatusUnauthorized, err)
		} else {
			writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
		}
		return
	}

	h.setSessionCookie(w, token, expiresAt, sessionMaxAge)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Logout(r.Context(), tokenFromContext(r.Context())); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	h.setSessionCookie(w, "", time.Unix(1, 0), -1)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, adminFromContext(r.Context()))
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func decodeCredentials(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("content type must be application/json"))
		return credentials{}, false
	}

	var value credentials
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return credentials{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return credentials{}, false
	}
	return value, true
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, value string, expires time.Time, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
