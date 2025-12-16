// Package auth provides authentication and authorization support.
// Authentication: You are who you say you are.
// Authorization:  You have permission to do what you are requesting to do.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/open-policy-agent/opa/v1/rego"
)

// Specific error variables for auth failures.
var (
	ErrForbidden       = errors.New("attempted action is not allowed")
	ErrKIDMissing      = errors.New("kid missing from token header")
	ErrKIDMalformed    = errors.New("kid in token header is malformed")
	ErrUserDisabled    = errors.New("user is disabled")
	ErrInvalidAuthOPA  = errors.New("OPA policy evaluation failed for authentication")
	ErrInvalidAuthzOPA = errors.New("OPA policy evaluation failed for authorization")
)

// Claims represents the authorization claims transmitted via a JWT.
type Claims struct {
	jwt.RegisteredClaims
	Admin     bool
	Endpoints map[string]bool
}

// KeyLookup declares a method set of behavior for looking up
// private and public keys for JWT use. The return could be a
// PEM encoded string or a JWS based key.
type KeyLookup interface {
	PrivateKey() (keyID string, pem string)
	PublicKey(kid string) (pem string, err error)
}

// Config represents information required to initialize auth.
type Config struct {
	KeyLookup KeyLookup
	Issuer    string
	Enabled   bool
}

// Auth is used to authenticate clients. It can generate a token for a
// set of user claims and recreate the claims by parsing the token.
type Auth struct {
	keyLookup KeyLookup
	method    jwt.SigningMethod
	parser    *jwt.Parser
	issuer    string
	enabled   bool
}

// New creates an Auth to support authentication/authorization.
func New(cfg Config) *Auth {
	return &Auth{
		keyLookup: cfg.KeyLookup,
		method:    jwt.GetSigningMethod(jwt.SigningMethodRS256.Name),
		parser:    jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Name})),
		issuer:    cfg.Issuer,
		enabled:   cfg.Enabled,
	}
}

// Issuer provides the configured issuer used to authenticate tokens.
func (a *Auth) Issuer() string {
	return a.issuer
}

// Enabled provides the configured issuer used to authenticate tokens.
func (a *Auth) Enabled() bool {
	return a.enabled
}

// GenerateToken generates a signed JWT token string representing the user Claims.
func (a *Auth) GenerateToken(claims Claims) (string, error) {
	kid, privateKeyPEM := a.keyLookup.PrivateKey()

	token := jwt.NewWithClaims(a.method, claims)
	token.Header["kid"] = kid

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return "", fmt.Errorf("parsing private key from PEM: %w", err)
	}

	str, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}

	return str, nil
}

// Authenticate processes the token to validate the sender's token is valid.
func (a *Auth) Authenticate(ctx context.Context, bearerToken string) (Claims, error) {
	if !strings.HasPrefix(bearerToken, "Bearer ") {
		return Claims{}, errors.New("expected authorization header format: Bearer <token>")
	}

	jwtUnverified := bearerToken[7:]

	var claims Claims
	token, _, err := a.parser.ParseUnverified(jwtUnverified, &claims)
	if err != nil {
		return Claims{}, fmt.Errorf("error parsing token: %w", err)
	}

	kidRaw, exists := token.Header["kid"]
	if !exists {
		return Claims{}, ErrKIDMissing
	}

	kid, ok := kidRaw.(string)
	if !ok {
		return Claims{}, ErrKIDMalformed
	}

	pem, err := a.keyLookup.PublicKey(kid)
	if err != nil {
		return Claims{}, fmt.Errorf("fetching public key for kid %q: %w", kid, err)
	}

	input := map[string]any{
		"Key":   pem,
		"Token": jwtUnverified,
		"ISS":   a.issuer,
	}

	if err := a.opaPolicyEvaluation(ctx, regoAuthentication, RuleAuthenticate, input, ErrInvalidAuthOPA); err != nil {
		return Claims{}, fmt.Errorf("authentication failed: token[%s] subject[%s]: %w", jwtUnverified, claims.Subject, err)
	}

	return claims, nil
}

// opaPolicyEvaluation asks opa to evaluate the token against the specified token
// policy and public key.
func (a *Auth) opaPolicyEvaluation(ctx context.Context, regoScript string, rule string, input any, baseError error) error {
	query := fmt.Sprintf("x = data.%s.%s", opaPackage, rule)

	q, err := rego.New(
		rego.Query(query),
		rego.Module("policy.rego", regoScript),
	).PrepareForEval(ctx)
	if err != nil {
		return fmt.Errorf("OPA prepare for eval failed for rule %q: %w", rule, err)
	}

	results, err := q.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return fmt.Errorf("OPA eval failed for rule %q: %w", rule, err)
	}

	if len(results) == 0 {
		return fmt.Errorf("%w: OPA policy evaluation for rule %q yielded no results", baseError, rule)
	}

	result, ok := results[0].Bindings["x"].(bool)
	if !ok || !result {
		return fmt.Errorf("%w: OPA policy rule %q not satisfied: results[%s] ok[%v]", baseError, rule, results, ok)
	}

	return nil
}
