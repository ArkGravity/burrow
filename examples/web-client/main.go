// A small confidential OIDC client for local interoperability testing.
// It intentionally uses a different OIDC client implementation from Burrow.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type pending struct {
	Nonce, Verifier string
	Expires         time.Time
}

func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func main() {
	issuer := os.Getenv("OIDC_ISSUER")
	if issuer == "" {
		issuer = "http://localhost:8080"
	}
	id, secret := os.Getenv("OIDC_CLIENT_ID"), os.Getenv("OIDC_CLIENT_SECRET")
	if id == "" || secret == "" {
		log.Fatal("set OIDC_CLIENT_ID and OIDC_CLIENT_SECRET")
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: 10 * time.Second})
	provider, e := oidc.NewProvider(ctx, issuer)
	if e != nil {
		log.Fatal(e)
	}
	endpoint := provider.Endpoint()
	endpoint.AuthStyle = oauth2.AuthStyleInHeader
	config := oauth2.Config{ClientID: id, ClientSecret: secret, Endpoint: endpoint, RedirectURL: "http://localhost:19001/callback", Scopes: []string{"openid", "profile", "email"}}
	verifier := provider.Verifier(&oidc.Config{ClientID: id})
	var mu sync.Mutex
	flows := map[string]pending{}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		state, nonce, pkce := token(), token(), oauth2.GenerateVerifier()
		mu.Lock()
		for k, v := range flows {
			if time.Now().After(v.Expires) {
				delete(flows, k)
			}
		}
		flows[state] = pending{nonce, pkce, time.Now().Add(5 * time.Minute)}
		mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "example_flow", Value: state, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})
		http.Redirect(w, r, config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(pkce)), http.StatusFound)
	})
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		cookie, e := r.Cookie("example_flow")
		if e != nil || cookie.Value != state {
			http.Error(w, "invalid state", 400)
			return
		}
		mu.Lock()
		flow, ok := flows[state]
		delete(flows, state)
		mu.Unlock()
		if !ok || time.Now().After(flow.Expires) {
			http.Error(w, "expired flow", 400)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "example_flow", Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
		tokens, e := config.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(flow.Verifier))
		if e != nil {
			http.Error(w, "exchange failed", 400)
			return
		}
		raw, ok := tokens.Extra("id_token").(string)
		if !ok {
			http.Error(w, "missing ID token", 400)
			return
		}
		identity, e := verifier.Verify(ctx, raw)
		if e != nil || identity.Nonce != flow.Nonce {
			http.Error(w, "invalid ID token", 400)
			return
		}
		if identity.AccessTokenHash != "" && identity.VerifyAccessToken(tokens.AccessToken) != nil {
			http.Error(w, "invalid access token hash", 400)
			return
		}
		var claims map[string]any
		if e = identity.Claims(&claims); e != nil {
			http.Error(w, "invalid claims", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{"message": "OIDC code + PKCE verified with coreos/go-oidc", "claims": claims})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><title>Burrow Web example</title><h1>Web client</h1><a href="/login">Sign in with Burrow</a>`)
	})
	log.Print("Web example: http://localhost:19001")
	log.Fatal((&http.Server{Addr: "127.0.0.1:19001", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
