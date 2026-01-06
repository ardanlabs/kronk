package dbsession

import "fmt"

var (
	New        = newStatus("new")
	Processing = newStatus("processing")
	Completed  = newStatus("completed")
	Error      = newStatus("error")
)

// =============================================================================

var statuses = make(map[string]Status)

type Status struct {
	value string
}

func newStatus(status string) Status {
	c := Status{status}
	statuses[status] = c
	return c
}

func (s Status) String() string {
	return s.value
}

func (s Status) Equal(s2 Status) bool {
	return s.value == s2.value
}

func (s *Status) UnmarshalText(text []byte) error {
	s.value = string(text)
	return nil
}

func (s Status) MarshalText() ([]byte, error) {
	return []byte(s.value), nil
}

// =============================================================================

func ParseStatus(value string) (Status, error) {
	choice, exists := statuses[value]
	if !exists {
		return Status{}, fmt.Errorf("invalid status value: '%s'. supported values 'none' and 'auto'", value)
	}

	return choice, nil
}

func MustParseStatus(value string) Status {
	choice, err := ParseStatus(value)
	if err != nil {
		panic(err)
	}

	return choice
}
