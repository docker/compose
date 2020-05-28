package amazon

//go:generate mockgen -destination=./api_mock.go -self_package "github.com/docker/ecs-plugin/pkg/amazon" -package=amazon . API

type API interface {
	downAPI
	upAPI
	logsAPI
	secretsAPI
	listAPI
}
