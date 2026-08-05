package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

func TestRateWindow(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    auth.RateWindow
		wantErr bool
	}{
		{"day", "day", auth.RateDay, false},
		{"month", "month", auth.RateMonth, false},
		{"year", "year", auth.RateYear, false},
		{"unlimited", "unlimited", auth.RateUnlimited, false},
		{"unknown", "unknown", auth.RateWindow{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := auth.ParseRateWindow(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRateWindow() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseRateWindow() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateWindowJSON(t *testing.T) {
	type config struct {
		Window auth.RateWindow `json:"window"`
	}

	data, err := json.Marshal(config{Window: auth.RateMonth})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	if got, want := string(data), `{"window":"month"}`; got != want {
		t.Errorf("json.Marshal() = %s, want %s", got, want)
	}

	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !cfg.Window.Equal(auth.RateMonth) {
		t.Errorf("json.Unmarshal() window = %q, want %q", cfg.Window, auth.RateMonth)
	}

	if err := json.Unmarshal([]byte(`{"window":"unknown"}`), &cfg); err == nil {
		t.Fatal("json.Unmarshal() error = nil, want error")
	}
}

func Test_Auth(t *testing.T) {
	ath, err := auth.New(auth.Config{
		KeyLookup: &keyStore{},
		Issuer:    "service project",
	})

	if err != nil {
		t.Fatalf("should be able to construct auth api : %s", err)
	}

	t.Run("authenticate", authenticate(ath))
	t.Run("authorize", authorize(ath))
}

func TestAuthenticateInvalidToken(t *testing.T) {
	ath, err := auth.New(auth.Config{
		KeyLookup: &keyStore{},
		Issuer:    "service project",
	})
	if err != nil {
		t.Fatalf("construct auth api: %v", err)
	}

	_, err = ath.Authenticate(context.Background(), "not a bearer token")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Authenticate: got %v, want ErrInvalidToken", err)
	}
}

func TestAuthenticateCanceledContext(t *testing.T) {
	ath, err := auth.New(auth.Config{
		KeyLookup: &keyStore{},
		Issuer:    "service project",
	})
	if err != nil {
		t.Fatalf("construct auth api: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = ath.Authenticate(ctx, "Bearer token")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Authenticate: got %v, want context.Canceled", err)
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Authenticate: got %v classified as invalid token, want internal cancellation", err)
	}
}

func authenticate(ath *auth.Auth) func(t *testing.T) {
	f := func(t *testing.T) {
		claims := auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    ath.Issuer(),
				Subject:   "5cf37266-3473-4006-984f-9325122678b7",
				ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			},
			Admin: true,
			Endpoints: map[string]auth.RateLimit{
				"chat-completions": {Limit: 0, Window: auth.RateUnlimited},
			},
		}

		token, err := ath.GenerateToken(claims)
		if err != nil {
			t.Fatalf("Should be able to generate a JWT : %s", err)
		}

		parsedClaims, err := ath.Authenticate(context.Background(), "Bearer "+token)
		if err != nil {
			t.Fatalf("Should be able to authenticate the claims : %s", err)
		}

		if _, err := uuid.Parse(claims.Subject); err != nil {
			t.Fatalf("Should be able to parse the subject : %s", err)
		}

		if parsedClaims.Subject != claims.Subject {
			t.Fatalf("Should be able to get back the same claims : %s", err)
		}
	}

	return f
}

func authorize(ath *auth.Auth) func(t *testing.T) {
	f := func(t *testing.T) {
		userClaims := auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:  "kronk project",
				Subject: "bill",
			},
			Admin: false,
			Endpoints: map[string]auth.RateLimit{
				"chat-completions": {Limit: 1000, Window: auth.RateDay},
			},
		}

		adminClaims := auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:  "kronk project",
				Subject: "admin",
			},
			Admin: true,
			Endpoints: map[string]auth.RateLimit{
				"chat-completions": {Limit: 0, Window: auth.RateUnlimited},
				"embeddings":       {Limit: 0, Window: auth.RateUnlimited},
			},
		}

		ctx := context.Background()

		// Admin tests
		t.Run("admin required with admin claims", func(t *testing.T) {
			err := ath.Authorize(ctx, adminClaims, true, "chat-completions")
			if err != nil {
				t.Fatalf("admin should be authorized: %s", err)
			}
		})

		t.Run("admin required with user claims", func(t *testing.T) {
			err := ath.Authorize(ctx, userClaims, true, "chat-completions")
			if err == nil {
				t.Fatal("user should not be authorized for admin")
			}
		})

		t.Run("admin not required with user claims", func(t *testing.T) {
			err := ath.Authorize(ctx, userClaims, false, "chat-completions")
			if err != nil {
				t.Fatalf("user should be authorized when admin not required: %s", err)
			}
		})

		// Endpoint tests
		t.Run("user allowed without endpoint restriction", func(t *testing.T) {
			err := ath.Authorize(ctx, userClaims, false, "")
			if err != nil {
				t.Fatalf("user should be authorized without endpoint restriction: %s", err)
			}
		})

		t.Run("user has endpoint", func(t *testing.T) {
			err := ath.Authorize(ctx, userClaims, false, "chat-completions")
			if err != nil {
				t.Fatalf("user should be authorized for chat-completions: %s", err)
			}
		})

		t.Run("user missing endpoint", func(t *testing.T) {
			err := ath.Authorize(ctx, userClaims, false, "embeddings")
			if err == nil {
				t.Fatal("user should not be authorized for embeddings")
			}
		})

		t.Run("admin has endpoint", func(t *testing.T) {
			err := ath.Authorize(ctx, adminClaims, false, "embeddings")
			if err != nil {
				t.Fatalf("admin should be authorized for embeddings: %s", err)
			}
		})

		t.Run("admin bypasses endpoint restrictions", func(t *testing.T) {
			err := ath.Authorize(ctx, adminClaims, false, "unknown-endpoint")
			if err != nil {
				t.Fatalf("admin should be authorized without endpoint permission: %s", err)
			}
		})
	}

	return f
}

// =============================================================================

type keyStore struct{}

func (ks *keyStore) PrivateKey() (string, string) {
	return kid, privateKeyPEM
}

func (ks *keyStore) PublicKey(kid string) (string, error) {
	return publicKeyPEM, nil
}

const (
	kid = "s4sKIjD9kIRjxs2tulPqGLdxSfgPErRN1Mu3Hd9k9NQ"

	privateKeyPEM = `-----BEGIN PRIVATE KEY-----
MIIEpQIBAAKCAQEAvMAHb0IoLvoYuW2kA+LTmnk+hfnBq1eYIh4CT/rMPCxgtzjq
U0guQOMnLg69ydyA5uu37v6rbS1+stuBTEiMQl/bxAhgLkGrUhgpZ10Bt6GzSEgw
QNloZoGaxe4p20wMPpT4kcMKNHkQds3uONNcLxPUmfjbbH64g+seg28pbgQPwKFK
tF7bIsOBgz0g5Ptn5mrkdzqMPUSy9k9VCu+R42LH9c75JsRzz4FeN+VzwMAL6yQn
ZvOi7/zOgNyxeVia8XVKykrnhgcpiOn5oaLRBzQGN00Z7TuBRIfDJWU21qQN4Cq7
keZmMP4gqCVWjYneK4bzrG/+H2w9BJ2TsmMGvwIDAQABAoIBAFQmQKpHkmavNYql
6POaksBRwaA1YzSijr7XJizGIXvKRSwqgb2zdnuTSgpspAx09Dr/aDdy7rZ0DAJt
fk2mInINDottOIQm3txwzTS58GQQAT/+fxTKWJMqwPfxYFPWqbbU76T8kXYna0Gs
OcK36GdMrgIfQqQyMs0Na8MpMg1LmkAxuqnFCXS/NMyKl9jInaaTS+Kz+BSzUMGQ
zebfLFsf2N7sLZuimt9zlRG30JJTfBlB04xsYMo734usA2ITe8U0XqG6Og0qc6ev
6lsoM8hpvEUsQLcjQQ5up7xx3S2stZJ8o0X8GEX5qUMaomil8mZ7X5xOlEqf7p+v
lXQ46cECgYEA2lbZQON6l3ZV9PCn9j1rEGaXio3SrAdTyWK3D1HF+/lEjClhMkfC
XrECOZYj+fiI9n+YpSog+tTDF7FTLf7VP21d2gnhQN6KAXUnLIypzXxodcC6h+8M
ZGJh/EydLvC7nPNoaXx96bohxzS8hrOlOlkCbr+8gPYKf8qkbe7HyxECgYEA3U6e
x9g4FfTvI5MGrhp2BIzoRSn7HlNQzjJ71iMHmM2kBm7TsER8Co1PmPDrP8K/UyGU
Q25usTsPSrHtKQEV6EsWKaP/6p2Q82sDkT9bZlV+OjRvOfpdO5rP6Q95vUmMGWJ/
S6oimbXXL8p3gDafw3vC1PCAhoaxMnGyKuZwlM8CgYEAixT1sXr2dZMg8DV4mMfI
8pqXf+AVyhWkzsz+FVkeyAKiIrKdQp0peI5C/5HfevVRscvX3aY3efCcEfSYKt2A
07WEKkdO4LahrIoHGT7FT6snE5NgfwTMnQl6p2/aVLNun20CHuf5gTBbIf069odr
Af7/KLMkjfWs/HiGQ6zuQjECgYEAv+DIvlDz3+Wr6dYyNoXuyWc6g60wc0ydhQo0
YKeikJPLoWA53lyih6uZ1escrP23UOaOXCDFjJi+W28FR0YProZbwuLUoqDW6pZg
U3DxWDrL5L9NqKEwcNt7ZIDsdnfsJp5F7F6o/UiyOFd9YQb7YkxN0r5rUTg7Lpdx
eMyv0/UCgYEAhX9MPzmTO4+N8naGFof1o8YP97pZj0HkEvM0hTaeAQFKJiwX5ijQ
xumKGh//G0AYsjqP02ItzOm2mWnbI3FrNlKmGFvR6VxIZMOyXvpLofHucjJ5SWli
eYjPklKcXaMftt1FVO4n+EKj1k1+Tv14nytq/J5WN+r4FBlNEYj/6vg=
-----END PRIVATE KEY-----
`
	publicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAvMAHb0IoLvoYuW2kA+LT
mnk+hfnBq1eYIh4CT/rMPCxgtzjqU0guQOMnLg69ydyA5uu37v6rbS1+stuBTEiM
Ql/bxAhgLkGrUhgpZ10Bt6GzSEgwQNloZoGaxe4p20wMPpT4kcMKNHkQds3uONNc
LxPUmfjbbH64g+seg28pbgQPwKFKtF7bIsOBgz0g5Ptn5mrkdzqMPUSy9k9VCu+R
42LH9c75JsRzz4FeN+VzwMAL6yQnZvOi7/zOgNyxeVia8XVKykrnhgcpiOn5oaLR
BzQGN00Z7TuBRIfDJWU21qQN4Cq7keZmMP4gqCVWjYneK4bzrG/+H2w9BJ2TsmMG
vwIDAQAB
-----END PUBLIC KEY-----
`
)
