package ardan.rego

import rego.v1

default auth := {"Authorized": false, "Reason": "unknown authorization failure"}

auth := {"Authorized": true, "Reason": ""} if {
	input.Requires.Admin
	input.Claim.Admin
}

auth := {"Authorized": true, "Reason": ""} if {
	not input.Requires.Admin
	endpoint_match
}

auth := {"Authorized": false, "Reason": "admin access required"} if {
	input.Requires.Admin
	not input.Claim.Admin
}

auth := {"Authorized": false, "Reason": sprintf("endpoint %q not authorized", [input.Requires.Endpoint])} if {
	not input.Requires.Admin
	not endpoint_match
}

endpoint_match if {
	input.Claim.Endpoints[input.Requires.Endpoint]
}
