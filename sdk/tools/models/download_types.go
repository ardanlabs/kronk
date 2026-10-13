package models

// pullKind selects the model body or its Git LFS pointer artifact.
type pullKind struct{ value int }

var (
	pullBody = pullKind{value: 0}
	pullSha  = pullKind{value: 1}
)
