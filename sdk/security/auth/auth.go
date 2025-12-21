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
	Admin     bool     `json:"admin"`
	Endpoints []string `json:"endpoints"`
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
}

// Auth is used to authenticate clients. It can generate a token for a
// set of user claims and recreate the claims by parsing the token.
type Auth struct {
	keyLookup         KeyLookup
	method            jwt.SigningMethod
	parser            *jwt.Parser
	issuer            string
	enabled           bool
	authenticateQuery rego.PreparedEvalQuery
	authorizeQuery    rego.PreparedEvalQuery
}

// New creates an Auth to support authentication/authorization.
func New(cfg Config) (*Auth, error) {
	const rule = "auth"
	query := fmt.Sprintf("x = data.%s.%s", opaPackage, rule)

	authenticateQuery, err := rego.New(
		rego.Query(query),
		rego.Module("policy.rego", regoAuthentication),
	).PrepareForEval(context.Background())
	if err != nil {
		return nil, fmt.Errorf("preparing authentication policy: %w", err)
	}

	authorizeQuery, err := rego.New(
		rego.Query(query),
		rego.Module("policy.rego", regoAuthorization),
	).PrepareForEval(context.Background())
	if err != nil {
		return nil, fmt.Errorf("preparing authorization policy: %w", err)
	}

	return &Auth{
		keyLookup:         cfg.KeyLookup,
		method:            jwt.GetSigningMethod(jwt.SigningMethodRS256.Name),
		parser:            jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Name})),
		issuer:            cfg.Issuer,
		authenticateQuery: authenticateQuery,
		authorizeQuery:    authorizeQuery,
	}, nil
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

	if err := a.opaAuthentication(ctx, input); err != nil {
		return Claims{}, fmt.Errorf("authentication failed: token[%s] subject[%s]: %w", jwtUnverified, claims.Subject, err)
	}

	return claims, nil
}

// Authorize checks if the claims have the required admin and endpoint permissions.
func (a *Auth) Authorize(ctx context.Context, claims Claims, requireAdmin bool, endpoint string) error {
	input := map[string]any{
		"Claim": map[string]any{
			"Admin":     claims.Admin,
			"Endpoints": claims.Endpoints,
		},
		"Requires": map[string]any{
			"Admin":    requireAdmin,
			"Endpoint": endpoint,
		},
	}

	result, err := a.opaAuthorization(ctx, input)
	if err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	if !result.Admin {
		return fmt.Errorf("%w: admin access required", ErrForbidden)
	}

	if !result.Endpoint {
		return fmt.Errorf("%w: endpoint %q not authorized", ErrForbidden, endpoint)
	}

	return nil
}

// =============================================================================

// opaAuthentication asks opa to evaluate the token against the specified token
// policy and public key.
func (a *Auth) opaAuthentication(ctx context.Context, input any) error {
	results, err := a.authenticateQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return fmt.Errorf("OPA eval failed: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("%w: no results", ErrInvalidAuthOPA)
	}

	resultMap, ok := results[0].Bindings["x"].(map[string]any)
	if !ok {
		return fmt.Errorf("%w: unexpected result type", ErrInvalidAuthOPA)
	}

	valid, _ := resultMap["valid"].(bool)
	if !valid {
		errMsg, _ := resultMap["error"].(string)
		return fmt.Errorf("%w: %s", ErrInvalidAuthOPA, errMsg)
	}

	return nil
}

type authResult struct {
	Admin    bool
	Endpoint bool
}

// opaAuthorization evaluates the authorization policy and returns the result document.
func (a *Auth) opaAuthorization(ctx context.Context, input any) (authResult, error) {
	results, err := a.authorizeQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return authResult{}, fmt.Errorf("OPA eval failed: %w", err)
	}

	if len(results) == 0 {
		return authResult{}, fmt.Errorf("%w: no results", ErrInvalidAuthzOPA)
	}

	resultMap, ok := results[0].Bindings["x"].(map[string]any)
	if !ok {
		return authResult{}, fmt.Errorf("%w: unexpected result type", ErrInvalidAuthzOPA)
	}

	adminVal, _ := resultMap["Admin"].(bool)
	endpointVal, _ := resultMap["Endpoint"].(bool)

	return authResult{
		Admin:    adminVal,
		Endpoint: endpointVal,
	}, nil
}
