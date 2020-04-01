package utils

const (
	LabelDockerComposePrefix = "com.docker.compose"
	LabelService             = LabelDockerComposePrefix + ".service"
	LabelVersion             = LabelDockerComposePrefix + ".version"
	LabelContainerNumber     = LabelDockerComposePrefix + ".container-number"
	LabelOneOff              = LabelDockerComposePrefix + ".oneoff"
	LabelNetwork             = LabelDockerComposePrefix + ".network"
	LabelSlug                = LabelDockerComposePrefix + ".slug"
	LabelVolume              = LabelDockerComposePrefix + ".volume"
	LabelConfigHash          = LabelDockerComposePrefix + ".config-hash"
	LabelProject             = LabelDockerComposePrefix + ".project"
	LabelWorkingDir          = LabelDockerComposePrefix + ".working_dir"
	LabelConfigFiles         = LabelDockerComposePrefix + ".config_files"
	LabelEnvironmentFile     = LabelDockerComposePrefix + ".environment_file"
)
