package main

import (
	"context"
	"crypto/tls"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/http-wasm/http-wasm-guest-tinygo/handler"
	"github.com/http-wasm/http-wasm-guest-tinygo/handler/api"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	_ "github.com/stealthrocket/net/http"
	"github.com/stealthrocket/net/wasip1"
	"golang.org/x/oauth2"
)

func main() {}

func init() {
	config := NewConfig()
	err := json.Unmarshal(handler.Host.GetConfig(), &config)
	if err != nil {
		handler.Host.Log(api.LogLevelError, fmt.Sprintf("Could not load config %v", err))
		os.Exit(1)
	}

	err = config.Validate()
	if err != nil {
		handler.Host.Log(api.LogLevelError, fmt.Sprintf("Invalid config %v", err))
		os.Exit(1)
	}

	mw, err := New(config)
	if err != nil {
		handler.Host.Log(api.LogLevelError, fmt.Sprintf("Could not init plugin %v", err))
		os.Exit(1)
	}
	handler.HandleRequestFn = mw.handleRequest
}

type TraefikOIDCWasm struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	totp     totp.ValidateOpts
	config   *Config
}

func New(config *Config) (*TraefikOIDCWasm, error) {
	if !config.Enable {
		return &TraefikOIDCWasm{config: config}, nil
	}
	ctx := context.Background()
	handler.Host.Log(api.LogLevelDebug, "initializing plugin")
	defer handler.Host.Log(api.LogLevelDebug, "initialized plugin")

	// Because there is no file mounted in the plugin by default, we configure insecureSkipVerify to avoid having to load rootCas
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec

	// Because there is no file mounted in the plugin by default, we configure a default resolver by config
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := wasip1.Dialer{
				Timeout: time.Millisecond * time.Duration(3000), //nolint:mnd
			}

			return d.DialContext(ctx, "udp", config.DNSAddr)
		},
	}

	provider, err := oidc.NewProvider(ctx, config.Provider.IssuerURL)
	if err != nil {
		return nil, err
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: config.Provider.ClientID,
	})

	return &TraefikOIDCWasm{
		config:   config,
		provider: provider,
		verifier: verifier,
		totp: totp.ValidateOpts{
			Period:    30, //nolint:mnd
			Skew:      1,
			Digits:    otp.DigitsEight,
			Algorithm: otp.AlgorithmSHA1,
		},
	}, nil
}

func (p *TraefikOIDCWasm) handleRequest(req api.Request, resp api.Response) (next bool, reqCtx uint32) {
	defer func() {
		if err := recover(); err != nil {
			handler.Host.Log(api.LogLevelError, fmt.Sprintf("panic recovered in handleRequest: %v", err))
			HttpError(resp, "Internal Server Error", http.StatusInternalServerError)
			next = false
			reqCtx = 0
		}
	}()

	if !p.config.Enable {
		return true, 0
	}
	if handler.Host.LogEnabled(api.LogLevelDebug) {
		handler.Host.Log(api.LogLevelDebug, "handle uri: "+req.GetURI())
	}

	_url, err := url.Parse(req.GetURI())
	if err != nil {
		handler.Host.Log(api.LogLevelError, "failed to parse uri: "+err.Error())
		return false, 0
	}
	if _url.Path == p.config.Endpoint.Callback {
		if err := p.handleCallback(req, resp); err != nil {
			handler.Host.Log(api.LogLevelError, "failed to handle callback: "+err.Error())
			HttpError(resp, err.Error(), http.StatusInternalServerError)
		}
		return false, 0
	} else if _url.Path == p.config.Endpoint.Logout {
		p.handleLogout(req, resp)
		return false, 0
	}
	if p.verifyToken(req, resp) {
		handler.Host.Log(api.LogLevelDebug, "token verified")
		return true, 0
	}
	handler.Host.Log(api.LogLevelDebug, "token not verified")
	p.doRedirect(req, resp)
	return false, 0
}

func (p *TraefikOIDCWasm) determineScheme(req api.Request) string {
	if scheme, _ := req.Headers().Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}

func (p *TraefikOIDCWasm) determineHost(req api.Request) string {
	if host, _ := req.Headers().Get("X-Forwarded-Host"); host != "" {
		return host
	}
	return ""
}

func (p *TraefikOIDCWasm) newOauth2Config(req api.Request) *oauth2.Config {
	redirectURL := fmt.Sprintf("%s://%s%s", p.determineScheme(req), p.determineHost(req), p.config.Endpoint.Callback)

	oauth2Config := &oauth2.Config{
		ClientID:     p.config.Provider.ClientID,
		ClientSecret: p.config.Provider.ClientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     p.provider.Endpoint(),
		Scopes:       p.config.Provider.Scopes,
	}

	return oauth2Config
}

func (p *TraefikOIDCWasm) doRedirect(req api.Request, resp api.Response) {
	oauth2Config := p.newOauth2Config(req)
	state, _ := totp.GenerateCodeCustom(base32.StdEncoding.EncodeToString([]byte(oauth2Config.ClientSecret)), time.Now(), p.totp)
	authCodeURL := oauth2Config.AuthCodeURL(state)
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.OriginPath, Value: req.GetURI(), Path: "/", HttpOnly: true})
	Redirect(req, resp, authCodeURL, http.StatusFound)
}

func (p *TraefikOIDCWasm) handleCallback(req api.Request, resp api.Response) error {
	ctx := context.Background()
	_url, err := url.Parse(req.GetURI())
	if err != nil {
		return errors.Join(errors.New("failed to parse url"), err)
	}

	oauth2Config := p.newOauth2Config(req)

	stateVerification, err := totp.ValidateCustom(_url.Query().Get("state"), base32.StdEncoding.EncodeToString([]byte(oauth2Config.ClientSecret)), time.Now(), p.totp)
	if err != nil {
		return errors.Join(errors.New("failed to verify state"), err)
	} else if !stateVerification {
		return errors.New("invalid state")
	}

	oauth2Token, err := oauth2Config.Exchange(ctx, _url.Query().Get("code"))
	if err != nil {
		return errors.Join(errors.New("failed to exchange token"), err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return errors.New("no id_token in token response")
	}
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return errors.Join(errors.New("failed to verify id_token"), err)
	}

	originPathCookie, err := GetCookie(req, p.config.Cookie.OriginPath)
	if err != nil {
		return errors.New("failed to get originPath cookie")
	}
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.AccessToken, Value: rawIDToken, Path: "/", HttpOnly: true, Expires: idToken.Expiry})
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.RefreshToken, Value: oauth2Token.RefreshToken, Path: "/", HttpOnly: true, Expires: oauth2Token.Expiry})
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.OriginPath, Value: "", Path: "/", HttpOnly: true, Expires: time.Now()})
	Redirect(req, resp, originPathCookie.Value, http.StatusFound)
	return nil
}

func (p *TraefikOIDCWasm) handleLogout(req api.Request, resp api.Response) {
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.AccessToken, Value: "", Path: "/", HttpOnly: true, Expires: time.Now()})
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.RefreshToken, Value: "", Path: "/", HttpOnly: true, Expires: time.Now()})
	Redirect(req, resp, p.config.Endpoint.Fallback, http.StatusFound)
}

func (p *TraefikOIDCWasm) verifyToken(req api.Request, resp api.Response) bool {
	tokenCookie, err := GetCookie(req, p.config.Cookie.AccessToken)
	if err != nil {
		return false
	}
	idToken, err := p.verifier.Verify(context.Background(), tokenCookie.Value)
	if err != nil {
		return false
	}

	if len(p.config.ClaimMap) > 0 {
		claims := make(map[string]any)
		_ = idToken.Claims(&claims)
		if handler.Host.LogEnabled(api.LogLevelDebug) {
			bs, _ := json.Marshal(claims)
			handler.Host.Log(api.LogLevelDebug, "claims: "+string(bs))
		}
		for claimName, headerName := range p.config.ClaimMap {
			if value, ok := claims[claimName]; ok {
				req.Headers().Add(headerName, fmt.Sprintf("%v", value))
			}
		}
	}

	if p.config.TokenAutoRefreshTime > 0 && time.Now().Add(p.config.TokenAutoRefreshTime).After(idToken.Expiry) {
		p.refreshToken(req, resp)
	}

	return true
}

func (p *TraefikOIDCWasm) refreshToken(req api.Request, resp api.Response) {
	refreshTokenCookie, err := GetCookie(req, p.config.Cookie.RefreshToken)
	if err != nil {
		return
	}
	oauth2Config := p.newOauth2Config(req)
	token, err := oauth2Config.TokenSource(context.Background(), &oauth2.Token{RefreshToken: refreshTokenCookie.Value}).Token()
	if err != nil {
		handler.Host.Log(api.LogLevelWarn, "failed to refresh token: "+err.Error())
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		handler.Host.Log(api.LogLevelWarn, "no id_token in token response")
		return
	}
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.AccessToken, Value: rawIDToken, Path: "/", HttpOnly: true, Expires: token.Expiry})
	SetCookie(resp, &http.Cookie{Name: p.config.Cookie.RefreshToken, Value: token.RefreshToken, Path: "/", HttpOnly: true, Expires: token.Expiry})

	handler.Host.Log(api.LogLevelDebug, "token refreshed")
}
