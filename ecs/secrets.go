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

package ecs

import (
	"context"

	"github.com/docker/compose-cli/api/secrets"
)

func (b *ecsAPIService) CreateSecret(ctx context.Context, secret secrets.Secret) (string, error) {
	return b.aws.CreateSecret(ctx, secret)
}

func (b *ecsAPIService) InspectSecret(ctx context.Context, id string) (secrets.Secret, error) {
	return b.aws.InspectSecret(ctx, id)
}

func (b *ecsAPIService) ListSecrets(ctx context.Context) ([]secrets.Secret, error) {
	return b.aws.ListSecrets(ctx)
}

func (b *ecsAPIService) DeleteSecret(ctx context.Context, id string, recover bool) error {
	return b.aws.DeleteSecret(ctx, id, recover)
}
