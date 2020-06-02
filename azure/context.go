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

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"

	"os"

	"github.com/AlecAivazis/survey/v2"

	"github.com/docker/api/context/store"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tj/survey/terminal"
)

func createContextData(ctx context.Context, opts map[string]string, selector userSelector) (interface{}, string, error) {
	var subscriptionID string
	if opts["aciSubscriptionID"] != "" {
		subscriptionID = opts["aciSubscriptionID"]
	} else {
		subs, err := getSubscriptionIDs(ctx)
		if err != nil {
			return nil, "", err
		}
		subscriptionID, err = chooseSub(subs, selector)
		if err != nil {
			return nil, "", err
		}
	}

	gc := getGroupsClient(subscriptionID)
	var group resources.Group
	var err error

	if opts["aciResourceGroup"] != "" {
		group, err = gc.Get(ctx, opts["aciResourceGroup"])
		if err != nil {
			return nil, "", errors.Wrapf(err, "Could not find resource group %q", opts["aciResourceGroup"])
		}
	} else {
		groupResponse, err := gc.List(ctx, "", nil)
		if err != nil {
			return nil, "", err
		}

		groups := groupResponse.Values()
		group, err = chooseGroup(ctx, gc, opts, groups, selector)
		if err != nil {
			return nil, "", err
		}
	}

	location := opts["aciLocation"]
	if location == "" {
		location = *group.Location
	}

	description := fmt.Sprintf("%s@%s", *group.Name, location)
	if opts["description"] != "" {
		description = fmt.Sprintf("%s (%s)", opts["description"], description)
	}

	return store.AciContext{
		SubscriptionID: subscriptionID,
		Location:       location,
		ResourceGroup:  *group.Name,
	}, description, nil
}

func createGroup(ctx context.Context, gc resources.GroupsClient, location string) (resources.Group, error) {
	if location == "" {
		location = "eastus"
	}
	gid := uuid.New().String()
	g, err := gc.CreateOrUpdate(ctx, gid, resources.Group{
		Location: &location,
	})
	if err != nil {
		return resources.Group{}, err
	}

	fmt.Printf("Resource group %q (%s) created\n", *g.Name, *g.Location)

	return g, nil
}

func chooseGroup(ctx context.Context, gc resources.GroupsClient, opts map[string]string, groups []resources.Group, selector userSelector) (resources.Group, error) {
	groupNames := []string{"create a new resource group"}
	for _, g := range groups {
		groupNames = append(groupNames, fmt.Sprintf("%s (%s)", *g.Name, *g.Location))
	}

	group, err := selector.userSelect("Choose a resource group", groupNames)
	if err != nil {
		if err == terminal.InterruptErr {
			os.Exit(0)
		}

		return resources.Group{}, err
	}

	if group == 0 {
		return createGroup(ctx, gc, opts["aciLocation"])
	}

	return groups[group-1], nil
}

func chooseSub(subs []subscription.Model, selector userSelector) (string, error) {
	if len(subs) == 1 {
		sub := subs[0]
		fmt.Println("Using only available subscription : " + *sub.DisplayName + "(" + *sub.SubscriptionID + ")")
		return *sub.SubscriptionID, nil
	}
	var options []string
	for _, sub := range subs {
		options = append(options, *sub.DisplayName+"("+*sub.SubscriptionID+")")
	}
	selected, err := selector.userSelect("Select a subscription ID", options)
	if err != nil {
		if err == terminal.InterruptErr {
			os.Exit(0)
		}
		return "", err
	}
	return *subs[selected].SubscriptionID, nil
}

type userSelector interface {
	userSelect(message string, options []string) (int, error)
}

type cliUserSelector struct{}

func (us cliUserSelector) userSelect(message string, options []string) (int, error) {
	qs := &survey.Select{
		Message: message,
		Options: options,
	}
	var selected int
	err := survey.AskOne(qs, &selected, nil)
	return selected, err
}
