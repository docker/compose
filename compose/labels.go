package compose

const (
	LABEL_DOCKER_COMPOSE_PREFIX = "com.docker.compose"
	LABEL_SERVICE               = LABEL_DOCKER_COMPOSE_PREFIX + ".service"
	LABEL_VERSION               = LABEL_DOCKER_COMPOSE_PREFIX + ".version"
	LABEL_CONTAINER_NUMBER      = LABEL_DOCKER_COMPOSE_PREFIX + ".container-number"
	LABEL_ONE_OFF               = LABEL_DOCKER_COMPOSE_PREFIX + ".oneoff"
	LABEL_NETWORK               = LABEL_DOCKER_COMPOSE_PREFIX + ".network"
	LABEL_SLUG                  = LABEL_DOCKER_COMPOSE_PREFIX + ".slug"
	LABEL_VOLUME                = LABEL_DOCKER_COMPOSE_PREFIX + ".volume"
	LABEL_CONFIG_HASH           = LABEL_DOCKER_COMPOSE_PREFIX + ".config-hash"
	LABEL_PROJECT               = LABEL_DOCKER_COMPOSE_PREFIX + ".project"
	LABEL_WORKING_DIR           = LABEL_DOCKER_COMPOSE_PREFIX + ".working_dir"
	LABEL_CONFIG_FILES          = LABEL_DOCKER_COMPOSE_PREFIX + ".config_files"
	LABEL_ENVIRONMENT_FILE      = LABEL_DOCKER_COMPOSE_PREFIX + ".environment_file"
)
