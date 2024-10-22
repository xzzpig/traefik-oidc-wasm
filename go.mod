module github.com/xzzpig/traefik-oidc-wasm

go 1.22.3

toolchain go1.22.4

require (
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/http-wasm/http-wasm-guest-tinygo v0.4.0
	github.com/pquerna/otp v1.4.0
	github.com/stealthrocket/net v0.2.1
	golang.org/x/net v0.21.0
	golang.org/x/oauth2 v0.23.0
)

require (
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/pquerna/cachecontrol v0.2.0 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	golang.org/x/crypto v0.27.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
)

replace github.com/http-wasm/http-wasm-guest-tinygo => github.com/traefik/http-wasm-guest-tinygo v0.0.0-20240913140402-af96219ffea5
