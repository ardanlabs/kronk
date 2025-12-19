package ardan.rego

import rego.v1

default auth := {"valid": false, "error": "signature_invalid"}

# Check for token expiration first (before full verification)
auth := {"valid": false, "error": "token_expired"} if {
	[_, payload, _] := io.jwt.decode(input.Token)
	now := time.now_ns() / 1000000000
	payload.exp < now
}

# Check for issuer mismatch
auth := {"valid": false, "error": "issuer_mismatch"} if {
	[_, payload, _] := io.jwt.decode(input.Token)
	payload.iss != input.ISS
}

# Full verification passes
auth := {"valid": true, "error": ""} if {
	[valid, _, _] := io.jwt.decode_verify(input.Token, {
		"cert": input.Key,
		"iss": input.ISS,
	})
	valid == true
}
