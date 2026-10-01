package ardan.rego

import rego.v1

default auth := {"valid": false, "error": "signature_invalid"}

auth := {"valid": true, "error": ""} if {
	[valid, _, payload] := io.jwt.decode_verify(input.Token, {
		"cert": input.Key,
		"iss": input.ISS,
		"alg": "RS256",
	})
	valid
	is_number(payload.exp)
}
