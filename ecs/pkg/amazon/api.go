package amazon

//go:generate mockgen -destination=./mock/api.go -package=mock . API

type API interface {
	downAPI
	upAPI
	logsAPI
	secretsAPI
}
