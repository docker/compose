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

package jobsv0

import (
	"github.com/containerd/errdefs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MapError translates the Jobs API's gRPC status codes into containerd/errdefs
// errors, so callers can reuse the same errdefs.IsXxx checks Compose already
// applies to the container/network/volume APIs. jobsv0.Jobs methods return
// raw gRPC errors (extensionclient.Resolve is lazy: unavailability only
// surfaces as codes.Unimplemented on the first real call), so callers must
// map each returned error explicitly.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.Unimplemented:
		return errdefs.ErrNotImplemented.WithMessage("the engine does not support jobs; start dockerd with --feature jobs")
	case codes.AlreadyExists:
		return errdefs.ErrAlreadyExists.WithMessage(st.Message())
	case codes.FailedPrecondition:
		return errdefs.ErrFailedPrecondition.WithMessage(st.Message())
	case codes.InvalidArgument:
		return errdefs.ErrInvalidArgument.WithMessage(st.Message())
	case codes.NotFound:
		return errdefs.ErrNotFound.WithMessage(st.Message())
	default:
		return err
	}
}
