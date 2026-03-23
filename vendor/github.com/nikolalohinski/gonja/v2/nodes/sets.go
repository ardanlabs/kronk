package nodes

import (
	"github.com/pkg/errors"
)

type BlockSet map[string]*Wrapper

// Exists returns true if the given block is already registered
func (bs BlockSet) Exists(name string) bool {
	_, existing := bs[name]
	return existing
}

// Register registers a new block. If there's already a filter with the same
// name, Register will panic. You usually want to call this
// function in the filter's init() function:
// http://golang.org/doc/effective_go.html#init
//
// See http://www.john-doe.de/post/gonja/ for more about
// writing filters and tags.
func (bs *BlockSet) Register(name string, w *Wrapper) error {
	if bs.Exists(name) {
		return errors.Errorf("Block with name '%s' is already registered", name)
	}
	(*bs)[name] = w
	return nil
}

// Replace replaces an already registered filter with a new implementation. Use this
// function with caution since it allobs you to change existing filter behaviour.
func (bs *BlockSet) Replace(name string, w *Wrapper) error {
	if !bs.Exists(name) {
		return errors.Errorf("Block with name '%s' does not exist (therefore cannot be overridden)", name)
	}
	(*bs)[name] = w
	return nil
}
