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

package resources

import (
	"context"
)

// PruneRequest options on what to prune
type PruneRequest struct {
	Force  bool
	DryRun bool
}

// PruneResult info on what has been pruned
type PruneResult struct {
	DeletedIDs []string
	Summary    string
}

// Service interacts with the underlying container backend
type Service interface {
	// Prune prune resources
	Prune(ctx context.Context, request PruneRequest) (PruneResult, error)
}
