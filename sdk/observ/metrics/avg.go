package metrics

import "expvar"

type avgMetric struct {
	sum   *expvar.Float
	count *expvar.Float
	min   *expvar.Float
	max   *expvar.Float
}

func newAvgMetric(name string) *avgMetric {
	a := &avgMetric{
		sum:   expvar.NewFloat(name + "_sum"),
		count: expvar.NewFloat(name + "_count"),
		min:   expvar.NewFloat(name + "_min"),
		max:   expvar.NewFloat(name + "_max"),
	}

	expvar.Publish(name+"_avg", expvar.Func(func() any {
		return a.average()
	}))

	return a
}

func (a *avgMetric) add(value float64) {
	a.sum.Add(value)
	a.count.Add(1)

	if a.count.Value() == 1 || value < a.min.Value() {
		a.min.Set(value)
	}

	if value > a.max.Value() {
		a.max.Set(value)
	}
}

func (a *avgMetric) average() float64 {
	c := a.count.Value()
	if c == 0 {
		return 0
	}

	return a.sum.Value() / c
}
