package metaservice

type Options struct {
	rawLeaves bool
}

type Option func(o *Options)

func newOptions(opts ...Option) *Options {
	options := Options{
		rawLeaves: false,
	}

	for _, o := range opts {
		o(&options)
	}

	return &options
}

func RawLeaves(rawLeaves bool) Option {
	return func(o *Options) {
		o.rawLeaves = rawLeaves
	}
}
