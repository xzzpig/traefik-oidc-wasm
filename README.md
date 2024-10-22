# Traefik OIDC WASM Plugin

This plugin allows you to secure the upstream services with an OpenID Connect (OIDC) provider. It uses the WASM extension of Traefik to perform.

> [!WARNING]
> This middleware is under active development - things should NOT break, but they might.

## 💡 Getting Started

Enable the plugin in your traefik configuration.

```yml
experimental:
  plugins:
    traefik-oidc:
      moduleName: "github.com/xzzpig/traefik-oidc-wasm"
      version: "v0.0.4"
```

Add a middleware and reference it in a route.

```yml
http:
  services:
    whoami:
      loadBalancer:
        servers:
          - url: http://whoami:80

  middlewares:
    oidc-auth:
      plugin:
        traefik-oidc:
          provider:
            issuerUrl: "https://idm.example.com"
            clientID: your_client_id
            clientSecret: your_client_secret
            scopes: ["openid", "profile", "email", "groups"]
          claimMap:
            name: "X-Oidc-Name"
            preferred_username: "X-Oidc-Username"
            sub: "X-Oidc-Subject"
            groups: "X-Oidc-Groups"
          endpoint:
            logout: "/oauth2/logout"

  routers:
    whoami:
      entryPoints: ["web"]
      rule: "HostRegexp(`.+`)"
      service: whoami
      middlewares: ["oidc-auth"]
```

## 🛠 Configuration Options
### Plugin Config
| Name | Required | Type | Default | Description |
|---|---|---|---|---|
| provider | yes | `Provider` | *none* | Identity Provider Configuration. See *Provider* Config. |
| cookie | no | `Cookie` | *none* | Cookie Configuration. See *Cookie* Config. |
| endpoint | no | `Endpoint` | *none* | Endpoint Configuration. See *Endpoint* Config. |
| totp | no | `TOTP` | *none* | TOTP Configuration to generate auth state. See *TOTP* Config. |
| claimMap | no | `map[string]string` | *none* | key value pairs of claims to extract from the OIDC token and set as headers. |
| dnsAddr | no | `string` | `"1.1.1.1:53"` | Address of the DNS server to use. (Because there is no default DNS resolver in WASM, this is required) |
| tokenAutoRefreshTime | no | `time.Duration` | `5m` | The rest of time to auto refresh the token. |
| enable | no | `bool` | `true` | Enable the plugin. |

### Provider Config
| Name | Required | Type | Default | Description |
|---|---|---|---|---|
| issuerUrl | yes | `string` | *none* | URL of the OIDC provider. |
| clientID | yes | `string` | *none* | Client ID of the OIDC client. |
| clientSecret | yes | `string` | *none* | Client Secret of the OIDC client. |
| scopes | no | `[]string` | `["openid"]` | Scopes to request from the OIDC provider. |

### Cookie Config
| Name | Required | Type | Default | Description |
|---|---|---|---|---|
| accessToken | no | `string` | `"__oidc_token"` | Name of the cookie to store the access token. |
| refreshToken | no | `string` | `"__oidc_refresh_token"` | Name of the cookie to store the refresh token. |
| originPath | no | `string` | `"__oidc_origin_path"` | Name of the cookie to store the origin path. |

### Endpoint Config
| Name | Required | Type | Default | Description |
|---|---|---|---|---|
| callback | no | `string` | `"/oauth2/callback"` | Path to the OIDC callback endpoint. |
| logout | no | `string` | `"/oauth2/logout"` | Path to the OIDC logout endpoint. |
| fallback | no | `string` | `"/"` | Path to the fallback endpoint. When logout is called, it will redirect to this endpoint. |

### TOTP Config
| Name | Required | Type | Default | Description |
|---|---|---|---|---|
| Period | no | `uint` | `30` | The period of the TOTP token. |
| Skew | no | `uint` | `0` | The skew of the TOTP token. |
| Digest | no | `uint` | `8` | The length of the TOTP token. |
| Algorithm | no | `string` | `"SHA1"` | The algorithm of the TOTP token. |