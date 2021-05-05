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

package aci

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/docker/compose-cli/api/context/store"
)

type contextMocks struct {
	userPrompt          *mockUserPrompt
	resourceGroupHelper *MockResourceGroupHelper
	contextCreateHelper contextCreateACIHelper
}

func testContextMocks() contextMocks {
	mockUserPrompt := &mockUserPrompt{}
	mockResourceGroupHelper := &MockResourceGroupHelper{}
	contextCreateHelper := contextCreateACIHelper{
		mockUserPrompt,
		mockResourceGroupHelper,
	}
	return contextMocks{mockUserPrompt, mockResourceGroupHelper, contextCreateHelper}
}

func TestCreateSpecifiedSubscriptionAndGroup(t *testing.T) {
	ctx := context.TODO()
	opts := options("1234", "myResourceGroup")
	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("1234", "Subscription1")}, nil)
	m.resourceGroupHelper.On("GetGroup", ctx, "1234", "myResourceGroup").Return(group("myResourceGroup", "eastus"), nil)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.NilError(t, err)
	assert.Equal(t, description, "myResourceGroup@eastus")
	assert.DeepEqual(t, data, aciContext("1234", "myResourceGroup", "eastus"))
}

func TestErrorOnNonExistentResourceGroup(t *testing.T) {
	ctx := context.TODO()
	opts := options("1234", "myResourceGroup")
	notFoundError := errors.New(`Not Found: "myResourceGroup"`)
	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("1234", "Subscription1")}, nil)
	m.resourceGroupHelper.On("GetGroup", ctx, "1234", "myResourceGroup").Return(resources.Group{}, notFoundError)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.Assert(t, cmp.Nil(data))
	assert.Equal(t, description, "")
	assert.Error(t, err, "Could not find resource group \"myResourceGroup\": Not Found: \"myResourceGroup\"")
}

func TestErrorOnNonExistentSubscriptionID(t *testing.T) {
	ctx := context.TODO()
	opts := options("otherSubscription", "myResourceGroup")
	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("1234", "Subscription1")}, nil)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.Assert(t, cmp.Nil(data))
	assert.Equal(t, description, "")
	assert.Assert(t, err == ErrSubscriptionNotFound)
}

func TestCreateNewResourceGroup(t *testing.T) {
	ctx := context.TODO()
	opts := options("1234", "")
	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("1234", "Subscription1")}, nil)
	m.resourceGroupHelper.On("GetGroup", ctx, "1234", "myResourceGroup").Return(group("myResourceGroup", "eastus"), nil)

	selectOptions := []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	m.userPrompt.On("Select", "Select a resource group", selectOptions).Return(0, nil)
	m.resourceGroupHelper.On("CreateOrUpdate", ctx, "1234", mock.AnythingOfType("string"), mock.AnythingOfType("resources.Group")).Return(group("newResourceGroup", "eastus"), nil)
	m.resourceGroupHelper.On("ListGroups", ctx, "1234").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.NilError(t, err)
	assert.Equal(t, description, "newResourceGroup@eastus")
	assert.DeepEqual(t, data, aciContext("1234", "newResourceGroup", "eastus"))
}

func TestCreateNewResourceGroupWithSpecificLocation(t *testing.T) {
	ctx := context.TODO()
	opts := options("1234", "")
	opts.Location = "eastus2"
	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("1234", "Subscription1")}, nil)
	m.resourceGroupHelper.On("GetGroup", ctx, "1234", "myResourceGroup").Return(group("myResourceGroup", "eastus"), nil)

	selectOptions := []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	m.userPrompt.On("Select", "Select a resource group", selectOptions).Return(0, nil)
	m.resourceGroupHelper.On("CreateOrUpdate", ctx, "1234", mock.AnythingOfType("string"), mock.AnythingOfType("resources.Group")).Return(group("newResourceGroup", "eastus"), nil)
	m.resourceGroupHelper.On("ListGroups", ctx, "1234").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.NilError(t, err)
	assert.Equal(t, description, "newResourceGroup@eastus2")
	assert.DeepEqual(t, data, aciContext("1234", "newResourceGroup", "eastus2"))
}

func TestSelectExistingResourceGroup(t *testing.T) {
	ctx := context.TODO()
	opts := options("1234", "")
	selectOptions := []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("1234", "Subscription1")}, nil)
	m.userPrompt.On("Select", "Select a resource group", selectOptions).Return(2, nil)
	m.resourceGroupHelper.On("ListGroups", ctx, "1234").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.NilError(t, err)
	assert.Equal(t, description, "group2@westeurope")
	assert.DeepEqual(t, data, aciContext("1234", "group2", "westeurope"))
}

func TestSelectSingleSubscriptionIdAndExistingResourceGroup(t *testing.T) {
	ctx := context.TODO()
	opts := options("", "")
	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("123456", "Subscription1")}, nil)

	selectOptions := []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	m.userPrompt.On("Select", "Select a resource group", selectOptions).Return(2, nil)
	m.resourceGroupHelper.On("ListGroups", ctx, "123456").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.NilError(t, err)
	assert.Equal(t, description, "group2@westeurope")
	assert.DeepEqual(t, data, aciContext("123456", "group2", "westeurope"))
}

func TestSelectSubscriptionIdAndExistingResourceGroup(t *testing.T) {
	ctx := context.TODO()
	opts := options("", "")
	sub1 := subModel("1234", "Subscription1")
	sub2 := subModel("5678", "Subscription2")

	m := testContextMocks()
	m.resourceGroupHelper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{sub1, sub2}, nil)

	selectOptions := []string{"Subscription1 (1234)", "Subscription2 (5678)"}
	m.userPrompt.On("Select", "Select a subscription ID", selectOptions).Return(1, nil)
	selectOptions = []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	m.userPrompt.On("Select", "Select a resource group", selectOptions).Return(2, nil)
	m.resourceGroupHelper.On("ListGroups", ctx, "5678").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := m.contextCreateHelper.createContextData(ctx, opts)
	assert.NilError(t, err)
	assert.Equal(t, description, "group2@westeurope")
	assert.DeepEqual(t, data, aciContext("5678", "group2", "westeurope"))
}

func subModel(subID string, display string) subscription.Model {
	return subscription.Model{
		SubscriptionID: to.StringPtr(subID),
		DisplayName:    to.StringPtr(display),
	}
}

func group(groupName string, location string) resources.Group {
	return resources.Group{
		Name:     to.StringPtr(groupName),
		Location: to.StringPtr(location),
	}
}

func aciContext(subscriptionID string, resourceGroupName string, location string) store.AciContext {
	return store.AciContext{
		SubscriptionID: subscriptionID,
		Location:       location,
		ResourceGroup:  resourceGroupName,
	}
}

func options(subscriptionID string, resourceGroupName string) ContextParams {
	return ContextParams{
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroupName,
	}
}

type mockUserPrompt struct {
	mock.Mock
}

func (s *mockUserPrompt) Select(message string, options []string) (int, error) {
	args := s.Called(message, options)
	return args.Int(0), args.Error(1)
}
func (s *mockUserPrompt) Confirm(message string, defaultValue bool) (bool, error) {
	args := s.Called(message, options)
	return args.Bool(0), args.Error(1)
}

func (s *mockUserPrompt) Input(message string, defaultValue string) (string, error) {
	args := s.Called(message, options)
	return args.String(0), args.Error(1)
}

func (s *mockUserPrompt) Password(message string) (string, error) {
	args := s.Called(message, options)
	return args.String(0), args.Error(1)
}

type MockResourceGroupHelper struct {
	mock.Mock
}

func (s *MockResourceGroupHelper) GetSubscriptionIDs(ctx context.Context) ([]subscription.Model, error) {
	args := s.Called(ctx)
	return args.Get(0).([]subscription.Model), args.Error(1)
}

func (s *MockResourceGroupHelper) ListGroups(ctx context.Context, subscriptionID string) ([]resources.Group, error) {
	args := s.Called(ctx, subscriptionID)
	return args.Get(0).([]resources.Group), args.Error(1)
}

func (s *MockResourceGroupHelper) GetGroup(ctx context.Context, subscriptionID string, groupName string) (resources.Group, error) {
	args := s.Called(ctx, subscriptionID, groupName)
	return args.Get(0).(resources.Group), args.Error(1)
}

func (s *MockResourceGroupHelper) CreateOrUpdate(ctx context.Context, subscriptionID string, resourceGroupName string, parameters resources.Group) (result resources.Group, err error) {
	args := s.Called(ctx, subscriptionID, resourceGroupName, parameters)
	return args.Get(0).(resources.Group), args.Error(1)
}

func (s *MockResourceGroupHelper) DeleteAsync(ctx context.Context, subscriptionID string, resourceGroupName string) (err error) {
	args := s.Called(ctx, subscriptionID, resourceGroupName)
	return args.Error(0)
}
