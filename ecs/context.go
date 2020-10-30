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
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"gopkg.in/ini.v1"

	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"
	"github.com/docker/compose-cli/prompt"
)

func getEnvVars() ContextParams {
	c := ContextParams{
		Profile: os.Getenv("AWS_PROFILE"),
		Region:  os.Getenv("AWS_REGION"),
	}
	if c.Region == "" {
		defaultRegion := os.Getenv("AWS_DEFAULT_REGION")
		if defaultRegion == "" {
			defaultRegion = "us-east-1"
		}
		c.Region = defaultRegion
	}

	p := credentials.EnvProvider{}
	creds, err := p.Retrieve()
	if err != nil {
		return c
	}
	c.AccessKey = creds.AccessKeyID
	c.SecretKey = creds.SecretAccessKey
	return c
}

type contextCreateAWSHelper struct {
	user prompt.UI
}

func newContextCreateHelper() contextCreateAWSHelper {
	return contextCreateAWSHelper{
		user: prompt.User{},
	}
}

func (h contextCreateAWSHelper) createContext(c *ContextParams) (interface{}, string) {
	if c.Profile == "default" {
		c.Profile = ""
	}
	var description string

	if c.CredsFromEnv {
		if c.Description == "" {
			description = "credentials read from environment"
		}
		return store.EcsContext{
			CredentialsFromEnv: c.CredsFromEnv,
			Profile:            c.Profile,
		}, description
	}

	if c.Region != "" {
		description = strings.TrimSpace(
			fmt.Sprintf("%s (%s)", c.Description, c.Region))
	}
	return store.EcsContext{
		Profile: c.Profile,
	}, description
}

func (h contextCreateAWSHelper) selectFromLocalProfile(opts *ContextParams) error {
	profilesList, err := getProfiles()
	if err != nil {
		return err
	}
	opts.Profile, err = h.chooseProfile(profilesList)
	return err
}

func (h contextCreateAWSHelper) createProfileFromCredentials(opts *ContextParams) error {
	accessKey, secretKey, err := h.askCredentials()
	if err != nil {
		return err
	}
	opts.AccessKey = accessKey
	opts.SecretKey = secretKey
	// we need a region set -- either read it from profile or prompt user
	// prompt for the region to use with this context
	opts.Region, err = h.chooseRegion(opts.Region, opts.Profile)
	if err != nil {
		return err
	}
	// save as a profile
	if opts.Profile == "" {
		opts.Profile = opts.Name
	}
	// check profile does not already exist
	profilesList, err := getProfiles()
	if err != nil {
		return err
	}
	if contains(profilesList, opts.Profile) {
		return fmt.Errorf("profile %q already exists", opts.Profile)
	}
	fmt.Printf("Saving to profile %q\n", opts.Profile)
	// context name used as profile name
	err = h.saveCredentials(opts.Name, opts.AccessKey, opts.SecretKey)
	if err != nil {
		return err
	}
	return h.saveRegion(opts.Name, opts.Region)
}

func (h contextCreateAWSHelper) createContextData(_ context.Context, opts ContextParams) (interface{}, string, error) {
	if opts.CredsFromEnv {
		ecsCtx, descr := h.createContext(&opts)
		return ecsCtx, descr, nil
	}
	options := []string{
		"An existing AWS profile",
		"A new AWS profile",
		"AWS environment variables",
	}

	selected, err := h.user.Select("Create a Docker context using:", options)
	if err != nil {
		if err == terminal.InterruptErr {
			return nil, "", errdefs.ErrCanceled
		}
		return nil, "", err
	}

	switch selected {
	case 0:
		err = h.selectFromLocalProfile(&opts)
	case 1:
		err = h.createProfileFromCredentials(&opts)
	case 2:
		opts.CredsFromEnv = true

	}
	if err != nil {
		return nil, "", err
	}
	ecsCtx, descr := h.createContext(&opts)
	return ecsCtx, descr, nil
}

func (h contextCreateAWSHelper) saveCredentials(profile string, accessKeyID string, secretAccessKey string) error {
	p := credentials.SharedCredentialsProvider{Profile: profile}
	_, err := p.Retrieve()
	if err == nil {
		return fmt.Errorf("credentials already exist")
	}

	if err.(awserr.Error).Code() == "SharedCredsLoad" && err.(awserr.Error).Message() == "failed to load shared credentials file" {
		_, err := os.Create(p.Filename)
		if err != nil {
			return err
		}
	}
	credIni, err := ini.Load(p.Filename)
	if err != nil {
		return err
	}
	section, err := credIni.NewSection(profile)
	if err != nil {
		return err
	}
	_, err = section.NewKey("aws_access_key_id", accessKeyID)
	if err != nil {
		return err
	}
	_, err = section.NewKey("aws_secret_access_key", secretAccessKey)
	if err != nil {
		return err
	}
	return credIni.SaveTo(p.Filename)
}

func (h contextCreateAWSHelper) saveRegion(profile, region string) error {
	if region == "" {
		return nil
	}
	// loads ~/.aws/config
	awsConfig := defaults.SharedConfigFilename()
	configIni, err := ini.Load(awsConfig)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		configIni = ini.Empty()
	}
	profile = fmt.Sprintf("profile %s", profile)
	section, err := configIni.GetSection(profile)
	if err != nil {
		if !strings.Contains(err.Error(), "does not exist") {
			return err
		}
		section, err = configIni.NewSection(profile)
		if err != nil {
			return err
		}
	}
	// save region under profile section in ~/.aws/config
	_, err = section.NewKey("region", region)
	if err != nil {
		return err
	}
	return configIni.SaveTo(awsConfig)
}

func getProfiles() ([]string, error) {
	profiles := []string{}
	// parse both .aws/credentials and .aws/config for profiles
	configFiles := map[string]bool{
		defaults.SharedCredentialsFilename(): false,
		defaults.SharedConfigFilename():      true,
	}
	for f, prefix := range configFiles {
		sections, err := loadIniFile(f, prefix)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for key := range sections {
			name := strings.ToLower(key)
			if !contains(profiles, name) {
				profiles = append(profiles, name)
			}
		}
	}
	return profiles, nil
}

func (h contextCreateAWSHelper) chooseProfile(profiles []string) (string, error) {
	options := []string{}
	options = append(options, profiles...)

	selected, err := h.user.Select("Select AWS Profile", options)
	if err != nil {
		if err == terminal.InterruptErr {
			return "", errdefs.ErrCanceled
		}
		return "", err
	}
	profile := options[selected]
	return profile, nil
}

func getRegion(profile string) (string, error) {
	if profile == "" {
		profile = "default"
	}
	// only load ~/.aws/config
	awsConfig := defaults.SharedConfigFilename()
	configIni, err := ini.Load(awsConfig)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		configIni = ini.Empty()
	}

	getProfileRegion := func(p string) string {
		r := ""
		section, err := configIni.GetSection(p)
		if err == nil {
			reg, err := section.GetKey("region")
			if err == nil {
				r = reg.Value()
			}
		}
		return r
	}
	if profile != "default" {
		profile = fmt.Sprintf("profile %s", profile)
	}
	region := getProfileRegion(profile)
	if region == "" {
		region = getProfileRegion("default")
		if region == "" {
			return "us-east-1", nil
		}
		return region, nil
	}
	return region, nil
}

func (h contextCreateAWSHelper) chooseRegion(region string, profile string) (string, error) {
	suggestion := region
	if suggestion == "" {
		region, err := getRegion(profile)
		if err != nil {
			return "", err
		}
		suggestion = region
	}
	// promp user for region
	region, err := h.user.Input("Region", suggestion)
	if err != nil {
		return "", err
	}
	if region == "" {
		return "", fmt.Errorf("region cannot be empty")
	}
	return region, nil
}

func (h contextCreateAWSHelper) askCredentials() (string, string, error) {
	accessKeyID, err := h.user.Input("AWS Access Key ID", "")
	if err != nil {
		return "", "", err
	}
	secretAccessKey, err := h.user.Password("Enter AWS Secret Access Key")
	if err != nil {
		return "", "", err
	}
	// validate access ID and password
	if len(accessKeyID) < 3 || len(secretAccessKey) < 3 {
		return "", "", fmt.Errorf("AWS Access/Secret Access Key must have more than 3 characters")
	}
	return accessKeyID, secretAccessKey, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func loadIniFile(path string, prefix bool) (map[string]ini.Section, error) {
	profiles := map[string]ini.Section{}
	credIni, err := ini.Load(path)
	if err != nil {
		return nil, err
	}
	for _, section := range credIni.Sections() {
		if prefix && strings.HasPrefix(section.Name(), "profile ") {
			profiles[section.Name()[len("profile "):]] = *section
		} else if !prefix || section.Name() == "default" {
			profiles[section.Name()] = *section
		}
	}
	return profiles, nil
}
