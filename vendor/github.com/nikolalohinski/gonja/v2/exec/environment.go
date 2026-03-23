package exec

import (
	"fmt"
	"maps"
	"reflect"
	"sync"

	"github.com/nikolalohinski/gonja/v2/parser"
	"github.com/pkg/errors"
)

type Environment struct {
	Filters           *FilterSet
	ControlStructures *ControlStructureSet
	Tests             *TestSet
	Context           *Context
	Methods           Methods
}

type FilterSet struct {
	filters map[string]FilterFunction
	lock    sync.Mutex
}

func NewFilterSet(filters map[string]FilterFunction) *FilterSet {
	return &FilterSet{
		filters: filters,
	}
}

// Exists returns true if the given filter is already registered
func (f *FilterSet) Exists(name string) bool {
	f.lock.Lock()
	defer f.lock.Unlock()
	_, existing := f.filters[name]
	return existing
}

// Get returns true and the named filter if it is already registered
func (f *FilterSet) Get(name string) (FilterFunction, bool) {
	f.lock.Lock()
	defer f.lock.Unlock()
	filter, ok := f.filters[name]
	return filter, ok
}

// Register registers a new filter. If there's already a filter with the same
// name, Register will panic. You usually want to call this
// function in the filter's init() function:
// http://golang.org/doc/effective_go.html#init
func (f *FilterSet) Register(name string, fn FilterFunction) error {
	if f.Exists(name) {
		return errors.Errorf("filter with name '%s' is already registered", name)
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	f.filters[name] = fn
	return nil
}

// Replace replaces an already registered filter with a new implementation. Use this
// function with caution since it allows you to change existing filter behaviour.
func (f *FilterSet) Replace(name string, fn FilterFunction) error {
	if !f.Exists(name) {
		return errors.Errorf("filter with name '%s' does not exist (therefore cannot be overridden)", name)
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	f.filters[name] = fn
	return nil
}

func (f *FilterSet) Update(other *FilterSet) *FilterSet {
	if other == nil {
		return f
	}
	f.lock.Lock()
	other.lock.Lock()
	defer f.lock.Unlock()
	defer other.lock.Unlock()
	maps.Copy(f.filters, other.filters)
	return f
}

type ControlStructureSet struct {
	statements map[string]parser.ControlStructureParser
	lock       sync.Mutex
}

func NewControlStructureSet(statements map[string]parser.ControlStructureParser) *ControlStructureSet {
	return &ControlStructureSet{
		statements: statements,
	}
}

// Exists returns true if the given test is already registered
func (c *ControlStructureSet) Exists(name string) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, existing := c.statements[name]
	return existing
}

func (c *ControlStructureSet) Get(name string) (parser.ControlStructureParser, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	parser, existing := c.statements[name]
	return parser, existing
}

// Register registers a new tag. You usually want to call this
// function in the tag's init() function:
// http://golang.org/doc/effective_go.html#init
func (c *ControlStructureSet) Register(name string, parser parser.ControlStructureParser) error {
	if c.Exists(name) {
		return errors.Errorf("ControlStructure '%s' is already registered", name)
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.statements[name] = parser
	return nil
}

// Replace replaces an already registered tag with a new implementation. Use this
// function with caution since it allows you to change existing tag behaviour.
func (c *ControlStructureSet) Replace(name string, parser parser.ControlStructureParser) error {
	if !c.Exists(name) {
		return errors.Errorf("ControlStructure '%s' does not exist (therefore cannot be overridden)", name)
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.statements[name] = parser
	return nil
}

func (c *ControlStructureSet) Update(other *ControlStructureSet) *ControlStructureSet {
	c.lock.Lock()
	defer c.lock.Unlock()
	other.lock.Lock()
	defer other.lock.Unlock()
	maps.Copy(c.statements, other.statements)
	return c
}

// TestSet maps test names to their TestFunction handler
type TestSet struct {
	tests map[string]TestFunction
	lock  sync.Mutex
}

func NewTestSet(tests map[string]TestFunction) *TestSet {
	return &TestSet{
		tests: tests,
	}
}

func (t *TestSet) validate(name string, fn TestFunction) error {
	testFn := reflect.ValueOf(fn)
	testType := testFn.Type()
	if testType.Kind() != reflect.Func || testType.NumIn() != 3 || testType.NumOut() != 2 {
		return fmt.Errorf("test '%s' is not a function with 3 arguments and 2 returns", name)
	}

	firstArgumentType := testType.In(0)
	switch firstArgumentType {
	case reflect.TypeFor[*Evaluator](), reflect.TypeFor[*Context]():
		// valid
	default:
		return fmt.Errorf("test '%s' first argument is neither of type *exec.Context nor *exec.Evaluator", name)
	}

	if testType.In(1) != reflect.TypeFor[*Value]() {
		return fmt.Errorf("test '%s' second argument is not of type *exec.Value", name)
	}
	if testType.In(2) != reflect.TypeFor[*VarArgs]() {
		return fmt.Errorf("test '%s' third argument is not of type *exec.VarArgs", name)
	}
	if testType.Out(0) != reflect.TypeFor[bool]() {
		return fmt.Errorf("test '%s' first returned variable is not of type bool", name)
	}
	if testType.Out(1) != reflect.TypeFor[error]() {
		return fmt.Errorf("test '%s' second returned variable is not of type error", name)
	}

	return nil
}

// Exists returns true if the given test is already registered
func (t *TestSet) Exists(name string) bool {
	t.lock.Lock()
	defer t.lock.Unlock()
	_, existing := t.tests[name]
	return existing
}

func (t *TestSet) Get(name string) (TestFunction, bool) {
	t.lock.Lock()
	defer t.lock.Unlock()
	fn, existing := t.tests[name]
	return fn, existing
}

// Register registers a new test. If there's already a test with the same
// name, RegisterTest will error out.
func (t *TestSet) Register(name string, fn TestFunction) error {
	if t.Exists(name) {
		return errors.Errorf("test with name '%s' is already registered", name)
	}
	t.lock.Lock()
	defer t.lock.Unlock()
	if err := t.validate(name, fn); err != nil {
		return err
	}
	t.tests[name] = fn
	return nil
}

// Replace replaces an already registered test with a new implementation. Use this
// function with caution since it allows you to change existing test behaviour.
func (t *TestSet) Replace(name string, fn TestFunction) error {
	if !t.Exists(name) {
		return errors.Errorf("test with name '%s' does not exist (therefore cannot be overridden)", name)
	}
	t.lock.Lock()
	defer t.lock.Unlock()
	if err := t.validate(name, fn); err != nil {
		return err
	}
	t.tests[name] = fn
	return nil
}

func (t *TestSet) Update(other *TestSet) *TestSet {
	t.lock.Lock()
	defer t.lock.Unlock()
	other.lock.Lock()
	defer other.lock.Unlock()
	for name, test := range other.tests {
		if err := t.validate(name, test); err != nil {
			continue
		}
		t.tests[name] = test
	}
	return t
}

type Method[I any] func(self I, selfValue *Value, arguments *VarArgs) (any, error)

type Methods struct {
	Bool  *MethodSet[bool]
	Int   *MethodSet[int]
	Float *MethodSet[float64]
	Str   *MethodSet[string]
	Dict  *MethodSet[map[string]any]
	List  *MethodSet[[]any]
}

type MethodSet[I any] struct {
	methods map[string]Method[I]
	lock    sync.Mutex
}

func NewMethodSet[I any](methods map[string]Method[I]) *MethodSet[I] {
	return &MethodSet[I]{
		methods: methods,
	}
}

func (m *MethodSet[I]) Get(name string) (Method[I], bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	method, existing := m.methods[name]
	return method, existing
}

func (m *MethodSet[I]) Exists(name string) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	_, existing := m.methods[name]
	return existing
}
