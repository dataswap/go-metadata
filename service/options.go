package metaservice

type Options struct {
	rawLeaves        bool   //Are the leaf nodes of the DAG of raw type?
	metaPath         string //paths for the mapping file and proof file.
	sourceParentPath string //Root directory of the source data.
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
