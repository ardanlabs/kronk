package apitest

// Table represent fields needed for running an api test.
type Table struct {
	Name       string
	SkipInGH   bool
	URL        string
	Token      string
	Headers    map[string]string
	Method     string
	StatusCode int
	Input      any
	GotResp    any
	ExpResp    any
	CmpFunc    func(got any, exp any) string
}
