/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package context

import (
	"context"
	"fmt"

	"github.com/docker/api/context/store"
)

func createACIContext(ctx context.Context, name string, opts createOpts) error {
	s := store.ContextStore(ctx)

	description := fmt.Sprintf("%s@%s", opts.aciResourceGroup, opts.aciLocation)
	if opts.description != "" {
		description = fmt.Sprintf("%s (%s)", opts.description, description)
	}

	return s.Create(
		name,
		store.AciContextType,
		description,
		store.AciContext{
			SubscriptionID: opts.aciSubscriptionID,
			Location:       opts.aciLocation,
			ResourceGroup:  opts.aciResourceGroup,
		},
	)
}
