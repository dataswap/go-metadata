package metaservice

type Options struct {
	rawLeaves        bool
	metaPath         string
	sourceParentPath string
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

func MetaPath(path string) Option {
	return func(o *Options) {
		o.metaPath = path
	}
}

func SourceParentPath(path string) Option {
	return func(o *Options) {
		o.sourceParentPath = path
	}
}
