package clustersconfig

type Rev interface {
	Rev() string
	SetRev(rev string)
}

type WithRev struct {
	rev string
}

func (r *WithRev) Rev() string {
	return r.rev
}

func (r *WithRev) SetRev(rev string) {
	r.rev = rev
}

var _ Rev = &WithRev{}
