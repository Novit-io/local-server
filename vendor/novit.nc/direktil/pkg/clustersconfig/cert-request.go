package clustersconfig

type CertRequest struct {
	Template `yaml:",inline"`

	CA      string
	Profile string
	Label   string
	PerHost bool `yaml:"per_host"`
}
