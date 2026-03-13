package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	pkgkioubit "example.com/webrtcserver/pkg/kioubit"
	pkglogin "example.com/webrtcserver/pkg/models/login"
	pkguser "example.com/webrtcserver/pkg/models/user"
	"github.com/google/uuid"
)

type KioubitLoginNonceState struct {
	SessionId   string
	CurrentPage string
}

type KioubitLoginHandler struct {
	nonceMap sync.Map

	// If this is empty, we would use default value (5m) for it.
	NonceLifespan time.Duration

	KioubitRedirURL string

	// If this is empty, we would use default value (see github docs) for it.
	KioubitLoginPage string

	LoginSuccessRedirectURL string
	UserManager             pkguser.UserManager
	UserSessionManager      pkglogin.UserSessionManager
	KioubitPubkey           []byte
}

func (h *KioubitLoginHandler) getKioubitLoginPage() string {
	if h.KioubitLoginPage != "" {
		return h.KioubitLoginPage
	}
	return "https://dn42.g-load.eu/auth/"
}

func (h *KioubitLoginHandler) getNonceLifespan() time.Duration {
	const defaultNonceLifespan = 5 * time.Minute
	if h.NonceLifespan == 0 {
		log.Printf("NonceLifespan is not set, using default: %+v", defaultNonceLifespan.String())
		return defaultNonceLifespan
	}
	return h.NonceLifespan
}

func (h *KioubitLoginHandler) createNonceFor(sessionId string, currentPage string) string {
	nonce := uuid.NewString()
	h.nonceMap.Store(nonce, &KioubitLoginNonceState{
		SessionId:   sessionId,
		CurrentPage: currentPage,
	})
	go func() {
		<-time.After(h.getNonceLifespan())
		if h != nil {
			h.nonceMap.Delete(nonce)
			log.Printf("Nonce %+v is deleted now", nonce)
		}
	}()
	return nonce
}

func (h *KioubitLoginHandler) getSessionIdByNonce(nonce string) string {
	if nonceState, ok := h.nonceMap.Load(nonce); ok && nonceState != nil {
		return nonceState.(*KioubitLoginNonceState).SessionId
	}
	return ""
}

func (h *KioubitLoginHandler) getInitialPageByNonce(nonce string) string {
	if nonceState, ok := h.nonceMap.Load(nonce); ok && nonceState != nil {
		return nonceState.(*KioubitLoginNonceState).CurrentPage
	}
	return ""
}

func (h *KioubitLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if url := r.URL; url != nil {
		if strings.HasSuffix(url.Path, "/login/start") {
			h.handleStart(w, r)
			return
		} else if strings.HasSuffix(url.Path, "/login/auth") {
			h.handleAuthorizationCode(w, r)
			return
		} else if strings.HasSuffix(url.Path, "/login/delete") {
			h.handleLogout(w, r)
			return
		}
	}
	h.handleNotFoundForThis(w, r)
}

func (h *KioubitLoginHandler) getKioubitRedirectURL(nonce string) string {
	urlVals := url.Values{}
	urlVals.Set("return", h.KioubitRedirURL)
	urlVals.Set("token", nonce)
	urlObj, err := url.Parse(h.getKioubitLoginPage())
	if err != nil {
		log.New(os.Stderr, "LoginHandler", 0).Println("Invalid github login page url:", err)
		return ""
	}
	urlObj.RawQuery = urlVals.Encode()
	return urlObj.String()
}

func (h *KioubitLoginHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionId := ctx.Value(CtxSessionKeySessionId)
	if sessionId == nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No session id is found"})
		return
	}
	nonce := h.createNonceFor(sessionId.(string), r.URL.Query().Get(QueryParamCurrentPage))
	redirURL := h.getKioubitRedirectURL(nonce)
	if redirURL == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to determine redir url (internal error)"})
		return
	}
	http.Redirect(w, r, redirURL, http.StatusTemporaryRedirect)
}

func (h *KioubitLoginHandler) handleAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sessionId := ctx.Value(CtxSessionKeySessionId)
	if sessionId == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No session id is found"})
		return
	}

	kioubitAuthCBParams, err := pkgkioubit.NewKioubitAuthCallbackParamsFromHTTPRequest(r, h.KioubitPubkey)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: err.Error()})
	}

	nonce := kioubitAuthCBParams.GetNonce()

	// See rfc6749 section-4.1.2 and section-4.1.2.1
	// https://datatracker.ietf.org/doc/html/rfc6749#section-4.1.2.1
	if err := r.URL.Query().Get("error"); err != "" {
		errDesc := r.URL.Query().Get("error_description")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("%s: %s", err, errDesc)})
		return
	}

	if s := h.getSessionIdByNonce(nonce); s == "" || s != sessionId.(string) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Invalid nonce %+v, no session is bound", nonce)})
		return
	}

	mnt := kioubitAuthCBParams.EffectiveMnt
	if mnt == "" && len(kioubitAuthCBParams.Mnt) > 0 {
		for _, m := range kioubitAuthCBParams.Mnt {
			if t := strings.TrimSpace(m); t != "" {
				mnt = t
				break
			}
		}
	}

	newUser := pkguser.User{
		Username:    mnt,
		DisplayName: kioubitAuthCBParams.EffectiveMnt,
		DN42ASN:     kioubitAuthCBParams.ASN,
	}
	originalUsername := newUser.Username

	const maxRetries = 5
	var userObject *pkguser.User

	for i := 0; i < maxRetries; i++ {
		userObject, _, err = h.UserManager.LoadOrCreateNewUserByDN42ASN(ctx, newUser.DN42ASN, newUser)
		if err == nil {
			break
		}
		if errors.Is(err, pkguser.ErrUsernameDuplicated) {
			// Generate random suffix (e.g., -a1b2) and retry
			randomSuffix := fmt.Sprintf("-%04x", rand.Intn(0x10000))
			newUser.Username = originalUsername + randomSuffix
			log.Printf("Username collision: original=%q, new attempt=%q, retry=%d/%d", originalUsername, newUser.Username, i+1, maxRetries)
			continue
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to load user from store"})
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to create user with unique username after maximum retries"})
		return
	}

	if err := h.UserSessionManager.LogIn(ctx, userObject.Id, sessionId.(string)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to log user in"})
		return
	}

	if initialPage := h.getInitialPageByNonce(nonce); initialPage != "" {
		http.Redirect(w, r, initialPage, http.StatusTemporaryRedirect)
		return
	}

	if u := h.LoginSuccessRedirectURL; u != "" {
		log.Printf("User %s has been successfully logged in, redirecting to %s", sessionId.(string), u)
		http.Redirect(w, r, u, http.StatusTemporaryRedirect)
		return
	}

	if originURL, err := url.Parse(r.URL.String()); err != nil && originURL != nil {
		originURL.RawFragment = ""
		originURL.RawQuery = ""
		originURL.RawPath = "/"
		log.Printf("User %s has been successfully logged in, redirecting to %s", sessionId.(string), originURL.String())
		http.Redirect(w, r, originURL.String(), http.StatusTemporaryRedirect)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *KioubitLoginHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessId := ctx.Value(CtxSessionKeySessionId)
	if sessId == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No valid session id is found"})
		return
	}

	if err := h.UserSessionManager.LogOut(ctx, sessId.(string)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Unable to log user out, internal problem"})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *KioubitLoginHandler) handleNotFoundForThis(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Path %s has no handler attached", r.URL.Path)})
}
