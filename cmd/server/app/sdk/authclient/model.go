package authclient

import "github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"

// AuthenticateReponse is the response for the auth service.
type AuthenticateReponse struct {
	Subject string
}

func toAuthenticateReponse(req *authapp.AuthenticateResponse) AuthenticateReponse {
	return AuthenticateReponse{
		Subject: req.GetSubject(),
	}
}

// CreateTokenResponse is the response for the auth service.
type CreateTokenResponse struct {
	Token string
}

func toCreateTokenResponse(req *authapp.CreateTokenResponse) CreateTokenResponse {
	return CreateTokenResponse{
		Token: req.GetToken(),
	}
}

// Key represents a key in the system.
type Key struct {
	ID      string
	Created string
}

// ListKeysResponse is the response for listing keys.
type ListKeysResponse struct {
	Keys []Key
}

func toListKeysResponse(req *authapp.ListKeysResponse) ListKeysResponse {
	keys := make([]Key, len(req.GetKeys()))
	for i, k := range req.GetKeys() {
		keys[i] = Key{
			ID:      k.GetId(),
			Created: k.GetCreated(),
		}
	}
	return ListKeysResponse{Keys: keys}
}
