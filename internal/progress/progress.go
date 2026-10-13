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

// Package progress builds the progress events Compose reports while it
// operates on resources.
package progress

import (
	"github.com/docker/compose/v5/pkg/api"
)

// New builds an event about a resource; reason, if given, is kept as its details.
func New(id string, status api.EventStatus, text string, reason ...string) api.Resource {
	r := api.Resource{
		ID:     id,
		Status: status,
		Text:   text,
	}
	if len(reason) > 0 {
		r.Details = reason[0]
	}
	return r
}

// Error builds the event of a resource whose operation failed.
func Error(id string, msg string) api.Resource {
	return api.Resource{
		ID:      id,
		Status:  api.Error,
		Text:    api.StatusError,
		Details: msg,
	}
}

// Creating is the event of a resource being created.
func Creating(id string) api.Resource {
	return New(id, api.Working, api.StatusCreating)
}

// Created is the event of a resource that was created.
func Created(id string) api.Resource {
	return New(id, api.Done, api.StatusCreated)
}

// Removing is the event of a resource being removed.
func Removing(id string) api.Resource {
	return New(id, api.Working, api.StatusRemoving)
}

// Removed is the event of a resource that was removed.
func Removed(id string) api.Resource {
	return New(id, api.Done, api.StatusRemoved)
}

// Stopping is the event of a resource being stopped.
func Stopping(id string) api.Resource {
	return New(id, api.Working, api.StatusStopping)
}

// Stopped is the event of a resource that was stopped.
func Stopped(id string) api.Resource {
	return New(id, api.Done, api.StatusStopped)
}
