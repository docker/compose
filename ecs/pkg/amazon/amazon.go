package amazon

import (
	"github.com/docker/ecs-plugin/pkg/amazon/backend"
	"github.com/docker/ecs-plugin/pkg/compose"
)

var _ compose.API = &backend.Backend{}
