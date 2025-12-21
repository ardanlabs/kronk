package authclient

import "github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"

// AuthenticateReponse is the response for the auth service.
type AuthenticateReponse struct {
	TokenID string
	Subject string
}

func toAuthenticateReponse(req *authapp.AuthenticateResponse) AuthenticateReponse {
	return AuthenticateReponse{
		TokenID: req.GetTokenId(),
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
