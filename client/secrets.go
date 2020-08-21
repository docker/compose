/*
   Copyright 2020 Docker, Inc.

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

package client

import (
	"context"

	"github.com/docker/compose-cli/errdefs"
	"github.com/docker/compose-cli/secrets"
)

type secretsService struct {
}

func (s *secretsService) CreateSecret(context.Context, secrets.Secret) (string, error) {
	return "", errdefs.ErrNotImplemented
}

func (s *secretsService) InspectSecret(context.Context, string) (secrets.Secret, error) {
	return secrets.Secret{}, errdefs.ErrNotImplemented
}

func (s *secretsService) ListSecrets(context.Context) ([]secrets.Secret, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *secretsService) DeleteSecret(context.Context, string, bool) error {
	return errdefs.ErrNotImplemented
}
