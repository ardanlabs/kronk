package ardan.rego

import rego.v1

default auth := {"Admin": true, "Endpoint": true}

auth := {"Admin": admin_check, "Endpoint": endpoint_check}

admin_check := true if {
	not input.Requires.Admin
}

admin_check := true if {
	input.Requires.Admin
	input.Claim.Admin
}

admin_check := false if {
	input.Requires.Admin
	not input.Claim.Admin
}

endpoint_check := true if {
	some ep in input.Claim.Endpoints
	ep == input.Requires.Endpoint
}

endpoint_check := false if {
	not endpoint_match
}

endpoint_match if {
	some ep in input.Claim.Endpoints
	ep == input.Requires.Endpoint
}
