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

package azure

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/docker/api/context/store"

	. "github.com/onsi/gomega"
)

type ContextSuiteTest struct {
	suite.Suite
	mockUserSelector       *MockUserSelector
	mockResourceGroupHeper *MockResourceGroupHelper
	contextCreateHelper    contextCreateACIHelper
}

func (suite *ContextSuiteTest) BeforeTest(suiteName, testName string) {
	suite.mockUserSelector = &MockUserSelector{}
	suite.mockResourceGroupHeper = &MockResourceGroupHelper{}
	suite.contextCreateHelper = contextCreateACIHelper{
		suite.mockUserSelector,
		suite.mockResourceGroupHeper,
	}
}

func (suite *ContextSuiteTest) TestCreateSpecifiedSubscriptionAndGroup() {
	ctx := context.TODO()
	opts := options("1234", "myResourceGroup")
	suite.mockResourceGroupHeper.On("GetGroup", ctx, "1234", "myResourceGroup").Return(group("myResourceGroup", "eastus"), nil)

	data, description, err := suite.contextCreateHelper.createContextData(ctx, opts)

	Expect(err).To(BeNil())
	Expect(description).To(Equal("myResourceGroup@eastus"))
	Expect(data).To(Equal(aciContext("1234", "myResourceGroup", "eastus")))
}

func (suite *ContextSuiteTest) TestErrorOnNonExistentResourceGroup() {
	ctx := context.TODO()
	opts := options("1234", "myResourceGroup")
	notFoundError := errors.New(`Not Found: "myResourceGroup"`)
	suite.mockResourceGroupHeper.On("GetGroup", ctx, "1234", "myResourceGroup").Return(resources.Group{}, notFoundError)

	data, description, err := suite.contextCreateHelper.createContextData(ctx, opts)

	Expect(data).To(BeNil())
	Expect(description).To(Equal(""))
	Expect(err.Error()).To(Equal("Could not find resource group \"myResourceGroup\": Not Found: \"myResourceGroup\""))
}

func (suite *ContextSuiteTest) TestCreateNewResourceGroup() {
	ctx := context.TODO()
	opts := options("1234", "")
	suite.mockResourceGroupHeper.On("GetGroup", ctx, "1234", "myResourceGroup").Return(group("myResourceGroup", "eastus"), nil)

	selectOptions := []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	suite.mockUserSelector.On("userSelect", "Select a resource group", selectOptions).Return(0, nil)
	suite.mockResourceGroupHeper.On("CreateOrUpdate", ctx, "1234", mock.AnythingOfType("string"), mock.AnythingOfType("resources.Group")).Return(group("newResourceGroup", "eastus"), nil)
	suite.mockResourceGroupHeper.On("ListGroups", ctx, "1234").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := suite.contextCreateHelper.createContextData(ctx, opts)

	Expect(err).To(BeNil())
	Expect(description).To(Equal("newResourceGroup@eastus"))
	Expect(data).To(Equal(aciContext("1234", "newResourceGroup", "eastus")))
}

func (suite *ContextSuiteTest) TestSelectExistingResourceGroup() {
	ctx := context.TODO()
	opts := options("1234", "")
	selectOptions := []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	suite.mockUserSelector.On("userSelect", "Select a resource group", selectOptions).Return(2, nil)
	suite.mockResourceGroupHeper.On("ListGroups", ctx, "1234").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := suite.contextCreateHelper.createContextData(ctx, opts)

	Expect(err).To(BeNil())
	Expect(description).To(Equal("group2@westeurope"))
	Expect(data).To(Equal(aciContext("1234", "group2", "westeurope")))
}

func (suite *ContextSuiteTest) TestSelectSingleSubscriptionIdAndExistingResourceGroup() {
	ctx := context.TODO()
	opts := options("", "")
	suite.mockResourceGroupHeper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{subModel("123456", "Subscription1")}, nil)

	selectOptions := []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	suite.mockUserSelector.On("userSelect", "Select a resource group", selectOptions).Return(2, nil)
	suite.mockResourceGroupHeper.On("ListGroups", ctx, "123456").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := suite.contextCreateHelper.createContextData(ctx, opts)

	Expect(err).To(BeNil())
	Expect(description).To(Equal("group2@westeurope"))
	Expect(data).To(Equal(aciContext("123456", "group2", "westeurope")))
}

func (suite *ContextSuiteTest) TestSelectSubscriptionIdAndExistingResourceGroup() {
	ctx := context.TODO()
	opts := options("", "")
	sub1 := subModel("1234", "Subscription1")
	sub2 := subModel("5678", "Subscription2")

	suite.mockResourceGroupHeper.On("GetSubscriptionIDs", ctx).Return([]subscription.Model{sub1, sub2}, nil)

	selectOptions := []string{"Subscription1 (1234)", "Subscription2 (5678)"}
	suite.mockUserSelector.On("userSelect", "Select a subscription ID", selectOptions).Return(1, nil)
	selectOptions = []string{"create a new resource group", "group1 (eastus)", "group2 (westeurope)"}
	suite.mockUserSelector.On("userSelect", "Select a resource group", selectOptions).Return(2, nil)
	suite.mockResourceGroupHeper.On("ListGroups", ctx, "5678").Return([]resources.Group{
		group("group1", "eastus"),
		group("group2", "westeurope"),
	}, nil)

	data, description, err := suite.contextCreateHelper.createContextData(ctx, opts)

	Expect(err).To(BeNil())
	Expect(description).To(Equal("group2@westeurope"))
	Expect(data).To(Equal(aciContext("5678", "group2", "westeurope")))
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
		Location:       "eastus",
	}
}

func TestContextSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(ContextSuiteTest))
}

type MockUserSelector struct {
	mock.Mock
}

func (s *MockUserSelector) userSelect(message string, options []string) (int, error) {
	args := s.Called(message, options)
	return args.Int(0), args.Error(1)
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
