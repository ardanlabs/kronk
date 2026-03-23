package builtins

import (
	methods "github.com/nikolalohinski/gonja/v2/builtins/methods"
	"github.com/nikolalohinski/gonja/v2/exec"
)

// Methods exports all builtins methods.
var Methods exec.Methods = methods.All
