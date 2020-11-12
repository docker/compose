/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: web PORT FOLDER")
		os.Exit(1)
	}

	http.HandleFunc("/failtestserver", log(fail))
	http.HandleFunc("/healthz", log(healthz))
	dir := os.Args[2]
	fileServer := http.FileServer(http.Dir(dir))
	http.HandleFunc("/", log(fileServer.ServeHTTP))

	port := os.Args[1]
	fmt.Println("Listening on port " + port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Error while starting server: %v", err)
	}
}

var healthy bool = true

func fail(w http.ResponseWriter, req *http.Request) {
	healthy = false
	fmt.Println("Server failing")
}

func healthz(w http.ResponseWriter, r *http.Request) {
	if !healthy {
		fmt.Println("unhealthy")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
}

func log(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
		handler(w, r)
	}
}
