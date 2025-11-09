// toProjectOptionsFns converts config options to cli.ProjectOptionsFn
func (o *configOptions) toProjectOptionsFns() []cli.ProjectOptionsFn {
	fns := []cli.ProjectOptionsFn{
		cli.WithInterpolation(!o.noInterpolate),
		cli.WithResolvedPaths(!o.noResolvePath),
		cli.WithNormalization(!o.noNormalize),
		cli.WithConsistency(!o.noConsistency),
		cli.WithDefaultProfiles(o.Profiles...),
		cli.WithDiscardEnvFile,
	}
	// Enable support for multiple 'extends' if the underlying compose-go library provides an option.
	// This assumes `cli.WithMultipleExtends()` is available in an updated compose-go version.
	fns = append(fns, cli.WithMultipleExtends())

	if o.noResolveEnv {
		fns = append(fns, cli.WithoutEnvironmentResolution)
	}
	return fns
}