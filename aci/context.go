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

package aci

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/prompt"
)

// ContextParams options for creating ACI context
type ContextParams struct {
	Description    string
	Location       string
	SubscriptionID string
	ResourceGroup  string
}

// ErrSubscriptionNotFound is returned when a required subscription is not found
var ErrSubscriptionNotFound = errors.New("subscription not found")

// IsSubscriptionNotFoundError returns true if the unwrapped error is IsSubscriptionNotFoundError
func IsSubscriptionNotFoundError(err error) bool {
	return errors.Is(err, ErrSubscriptionNotFound)
}

type contextCreateACIHelper struct {
	selector            prompt.UI
	resourceGroupHelper ResourceGroupHelper
}

func newContextCreateHelper() contextCreateACIHelper {
	return contextCreateACIHelper{
		selector:            prompt.User{},
		resourceGroupHelper: aciResourceGroupHelperImpl{},
	}
}

func (helper contextCreateACIHelper) createContextData(ctx context.Context, opts ContextParams) (interface{}, string, error) {
	subs, err := helper.resourceGroupHelper.GetSubscriptionIDs(ctx)
	if err != nil {
		return nil, "", err
	}
	subscriptionID := ""
	if opts.SubscriptionID != "" {
		for _, sub := range subs {
			if *sub.SubscriptionID == opts.SubscriptionID {
				subscriptionID = opts.SubscriptionID
			}
		}
		if subscriptionID == "" {
			return nil, "", ErrSubscriptionNotFound
		}
	} else {
		subscriptionID, err = helper.chooseSub(subs)
		if err != nil {
			return nil, "", err
		}
	}

	var group resources.Group
	if opts.ResourceGroup != "" {
		group, err = helper.resourceGroupHelper.GetGroup(ctx, subscriptionID, opts.ResourceGroup)
		if err != nil {
			return nil, "", errors.Wrapf(err, "Could not find resource group %q", opts.ResourceGroup)
		}
	} else {
		groups, err := helper.resourceGroupHelper.ListGroups(ctx, subscriptionID)
		if err != nil {
			return nil, "", err
		}
		group, err = helper.chooseGroup(ctx, subscriptionID, opts, groups)
		if err != nil {
			return nil, "", err
		}
	}

	location := *group.Location

	description := fmt.Sprintf("%s@%s", *group.Name, location)
	if opts.Description != "" {
		description = fmt.Sprintf("%s (%s)", opts.Description, description)
	}

	return store.AciContext{
		SubscriptionID: subscriptionID,
		Location:       location,
		ResourceGroup:  *group.Name,
	}, description, nil
}

func (helper contextCreateACIHelper) createGroup(ctx context.Context, subscriptionID, location string) (resources.Group, error) {
	if location == "" {
		location = "eastus"
	}
	gid := uuid.New().String()
	g, err := helper.resourceGroupHelper.CreateOrUpdate(ctx, subscriptionID, gid, resources.Group{
		Location: &location,
	})
	if err != nil {
		return resources.Group{}, err
	}

	fmt.Printf("Resource group %q (%s) created\n", *g.Name, *g.Location)

	return g, nil
}

func (helper contextCreateACIHelper) chooseGroup(ctx context.Context, subscriptionID string, opts ContextParams, groups []resources.Group) (resources.Group, error) {
	groupNames := []string{"create a new resource group"}
	for _, g := range groups {
		groupNames = append(groupNames, fmt.Sprintf("%s (%s)", *g.Name, *g.Location))
	}

	group, err := helper.selector.Select("Select a resource group", groupNames)
	if err != nil {
		if err == terminal.InterruptErr {
			os.Exit(0)
		}

		return resources.Group{}, err
	}

	if group == 0 {
		return helper.createGroup(ctx, subscriptionID, opts.Location)
	}

	return groups[group-1], nil
}

func (helper contextCreateACIHelper) chooseSub(subs []subscription.Model) (string, error) {
	if len(subs) == 1 {
		sub := subs[0]
		fmt.Println("Using only available subscription : " + display(sub))
		return *sub.SubscriptionID, nil
	}
	var options []string
	for _, sub := range subs {
		options = append(options, display(sub))
	}
	selected, err := helper.selector.Select("Select a subscription ID", options)
	if err != nil {
		if err == terminal.InterruptErr {
			os.Exit(0)
		}
		return "", err
	}

	return *subs[selected].SubscriptionID, nil
}

func display(sub subscription.Model) string {
	return fmt.Sprintf("%s (%s)", *sub.DisplayName, *sub.SubscriptionID)
}
