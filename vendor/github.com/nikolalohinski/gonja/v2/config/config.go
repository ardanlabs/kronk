// Package config provides configuration types for the template engine.
package config

// Config holds plexer and parser parameters
type Config struct {
	// The string marking the beginning of a block. Defaults to '{%'
	BlockStartString string
	// The string marking the end of a block. Defaults to '%}'.
	BlockEndString string
	// The string marking the beginning of a print controlStructure. Defaults to '{{'.
	VariableStartString string
	// The string marking the end of a print controlStructure. Defaults to '}}'.
	VariableEndString string
	// The string marking the beginning of a comment. Defaults to '{#'.
	CommentStartString string
	// The string marking the end of a comment. Defaults to '#}'.
	CommentEndString string
	// If set to True the XML/HTML autoescaping feature is enabled by default.
	// For more details about autoescaping see Markup.
	// This can also be a callable that is passed the template name
	// and has to return True or False depending on autoescape should be enabled by default.
	AutoEscape bool
	// Whether to be strict about undefined attribute or item in an object and return error
	// or return a nil value on missing data and ignore it entirely
	StrictUndefined bool
	// If is set to true, the first newline after a block is removed (block, not variable !tag)
	TrimBlocks bool
	// If is set to true, the leading spaces and tabes are stripped from the start of a line to a block
	LeftStripBlocks bool
}

func New() *Config {
	return &Config{
		BlockStartString:    "{%",
		BlockEndString:      "%}",
		VariableStartString: "{{",
		VariableEndString:   "}}",
		CommentStartString:  "{#",
		CommentEndString:    "#}",
		AutoEscape:          false,
		StrictUndefined:     false,
		TrimBlocks:          false,
		LeftStripBlocks:     false,
	}
}

func (c *Config) Inherit() *Config {
	return &Config{
		BlockStartString:    c.BlockStartString,
		BlockEndString:      c.BlockEndString,
		VariableStartString: c.VariableStartString,
		VariableEndString:   c.VariableEndString,
		CommentStartString:  c.CommentStartString,
		CommentEndString:    c.CommentEndString,
		AutoEscape:          c.AutoEscape,
		StrictUndefined:     c.StrictUndefined,
		TrimBlocks:          c.TrimBlocks,
		LeftStripBlocks:     c.LeftStripBlocks,
	}
}
