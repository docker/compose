package login

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
)

const loginFailedHTML = `
	<!DOCTYPE html>
	<html>
	<head>
	    <meta charset="utf-8" />
	    <title>Login failed</title>
	</head>
	<body>
	    <h4>Some failures occurred during the authentication</h4>
	    <p>You can log an issue at <a href="https://github.com/azure/azure-cli/issues">Azure CLI GitHub Repository</a> and we will assist you in resolving it.</p>
	</body>
	</html>
	`

const successfullLoginHTML = `
	<!DOCTYPE html>
	<html>
	<head>
	    <meta charset="utf-8" />
	    <meta http-equiv="refresh" content="10;url=https://docs.microsoft.com/cli/azure/">
	    <title>Login successfully</title>
	</head>
	<body>
	    <h4>You have logged into Microsoft Azure!</h4>
	    <p>You can close this window, or we will redirect you to the <a href="https://docs.microsoft.com/cli/azure/">Azure CLI documents</a> in 10 seconds.</p>
	</body>
	</html>
	`

func startLoginServer(queryCh chan url.Values) (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", queryHandler(queryCh))
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}

	availablePort := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil {
			queryCh <- url.Values{
				"error": []string{fmt.Sprintf("error starting http server with: %v", err)},
			}
		}
	}()
	return availablePort, nil
}

func queryHandler(queryCh chan url.Values) func(w http.ResponseWriter, r *http.Request) {
	queryHandler := func(w http.ResponseWriter, r *http.Request) {
		_, hasCode := r.URL.Query()["code"]
		if hasCode {
			_, err := w.Write([]byte(successfullLoginHTML))
			if err != nil {
				queryCh <- url.Values{
					"error": []string{err.Error()},
				}
			} else {
				queryCh <- r.URL.Query()
			}
		} else {
			_, err := w.Write([]byte(loginFailedHTML))
			if err != nil {
				queryCh <- url.Values{
					"error": []string{err.Error()},
				}
			} else {
				queryCh <- r.URL.Query()
			}
		}
	}
	return queryHandler
}
