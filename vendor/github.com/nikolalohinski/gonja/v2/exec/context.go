package exec

import "maps"

import "sync"

type Context struct {
	data   map[string]any
	parent *Context
	lock   sync.Mutex
}

func NewContext(data map[string]any) *Context {
	return &Context{data: data}
}

func EmptyContext() *Context {
	return &Context{data: map[string]any{}}
}

func (ctx *Context) Has(name string) bool {
	ctx.lock.Lock()
	_, exists := ctx.data[name]
	ctx.lock.Unlock()
	if !exists && ctx.parent != nil {
		return ctx.parent.Has(name)
	}
	return exists
}

func (ctx *Context) Get(name string) (any, bool) {
	ctx.lock.Lock()
	value, exists := ctx.data[name]
	ctx.lock.Unlock()
	if exists {
		return value, true
	} else if ctx.parent != nil {
		return ctx.parent.Get(name)
	} else {
		return nil, false
	}
}

func (ctx *Context) Set(name string, value any) {
	ctx.lock.Lock()
	ctx.data[name] = value
	ctx.lock.Unlock()
}

func (ctx *Context) Inherit() *Context {
	ctx.lock.Lock()
	inherited := &Context{
		data:   map[string]any{},
		parent: ctx,
	}
	ctx.lock.Unlock()
	return inherited
}

// Update updates this context with the key/value pairs from a map.
func (ctx *Context) Update(other *Context) *Context {
	if other == nil {
		return ctx
	}
	ctx.lock.Lock()
	maps.Copy(ctx.data, other.data)
	ctx.lock.Unlock()
	return ctx
}
