// +build !ecs

package mobycli

import "github.com/docker/api/context/store"

func init() {
	delegatedContextTypes = append(delegatedContextTypes, store.AwsContextType, store.EcsContextType)
}
