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

type contextElements struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
	Profile      string
	Region       string
	CredsFromEnv bool
}

func (c contextElements) HaveRequiredCredentials() bool {
	if c.AccessKey != "" && c.SecretKey != "" {
		return true
	}
	return false
}

type contextCreateAWSHelper struct {
	user prompt.UI
}

func newContextCreateHelper() contextCreateAWSHelper {
	return contextCreateAWSHelper{
		user: prompt.User{},
	}
}

func getEnvVars() contextElements {
	c := contextElements{}
	profile := os.Getenv("AWS_PROFILE")
	if profile != "" {
		c.Profile = profile
	}

	p := credentials.EnvProvider{}
	creds, err := p.Retrieve()
	if err != nil {
		return c
	}
	c.AccessKey = creds.AccessKeyID
	c.SecretKey = creds.SecretAccessKey
	c.SessionToken = creds.SessionToken
	return c
}

func (h contextCreateAWSHelper) createProfile(name string, c *contextElements) error {
	if c != nil {
		if c.AccessKey != "" && c.SecretKey != "" {
			return h.saveCredentials(name, c.AccessKey, c.SecretKey)
		}
		accessKey, secretKey, err := h.askCredentials()

		if err != nil {
			return err
		}
		c.AccessKey = accessKey
		c.SecretKey = secretKey
		return h.saveCredentials(name, c.AccessKey, c.SecretKey)
	}

	accessKey, secretKey, err := h.askCredentials()
	if err != nil {
		return err
	}
	if accessKey != "" && secretKey != "" {
		return h.saveCredentials(name, accessKey, secretKey)
	}
	return nil
}

func (h contextCreateAWSHelper) createContext(c *contextElements, description string) (interface{}, string) {
	if c.Profile == "default" {
		c.Profile = ""
	}
	description = strings.TrimSpace(
		fmt.Sprintf("%s (%s)", description, c.Region))
	if c.CredsFromEnv {
		return store.EcsContext{
			CredentialsFromEnv: c.CredsFromEnv,
			Profile:            c.Profile,
			Region:             c.Region,
		}, description
	}
	return store.EcsContext{
		Profile: c.Profile,
		Region:  c.Region,
	}, description
}

func (h contextCreateAWSHelper) createContextData(_ context.Context, opts ContextParams) (interface{}, string, error) {
	creds := contextElements{}

	options := []string{
		"Use AWS credentials set via environment variables",
		"Create a new profile with AWS credentials",
		"Select from existing local AWS profiles",
	}
	//if creds.HaveRequiredProps() {
	selected, err := h.user.Select("Would you like to create your context based on", options)
	if err != nil {
		if err == terminal.InterruptErr {
			return nil, "", errdefs.ErrCanceled
		}
		return nil, "", err
	}
	if creds.Region == "" {
		creds.Region = opts.Region
	}
	if creds.Profile == "" {
		creds.Profile = opts.Profile
	}

	switch selected {
	case 0:
		creds.CredsFromEnv = true
		// confirm region profile should target
		if creds.Region == "" {
			creds.Region, err = h.chooseRegion(creds.Region, creds.Profile)
			if err != nil {
				return nil, "", err
			}
		}
	case 1:
		accessKey, secretKey, err := h.askCredentials()
		if err != nil {
			return nil, "", err
		}
		creds.AccessKey = accessKey
		creds.SecretKey = secretKey
		// we need a region set -- either read it from profile or prompt user

		// prompt for the region to use with this context
		creds.Region, err = h.chooseRegion(creds.Region, creds.Profile)
		if err != nil {
			return nil, "", err
		}
		// save as a profile
		if creds.Profile == "" {
			creds.Profile = opts.Name
		}
		fmt.Printf("Saving credentials under profile %s\n", creds.Profile)
		h.createProfile(creds.Profile, &creds)

	case 2:
		profilesList, err := h.getProfiles()
		if err != nil {
			return nil, "", err
		}
		// choose profile
		creds.Profile, err = h.chooseProfile(profilesList)
		if err != nil {
			return nil, "", err
		}
		if creds.Region == "" {
			creds.Region, err = h.chooseRegion(creds.Region, creds.Profile)
			if err != nil {
				return nil, "", err
			}
		}
	}

	ecsCtx, descr := h.createContext(&creds, opts.Description)
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

func (h contextCreateAWSHelper) getProfiles() ([]string, error) {
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

func (h contextCreateAWSHelper) getRegionSuggestion(region string, profile string) (string, error) {
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

	var f func(string, string) string
	f = func(r string, p string) string {
		section, err := configIni.GetSection(p)
		if err == nil {
			reg, err := section.GetKey("region")
			if err == nil {
				r = reg.Value()
			}
		}
		if r == "" {
			switch p {
			case "":
				return "us-east-1"
			case "default":
				return f(r, "")
			}
			return f(r, "default")
		}
		return r
	}

	if profile != "default" {
		profile = fmt.Sprintf("profile %s", profile)
	}
	return f(region, profile), nil
}

func (h contextCreateAWSHelper) chooseRegion(region string, profile string) (string, error) {
	suggestion, err := h.getRegionSuggestion(region, profile)
	if err != nil {
		return "", err
	}
	// promp user for region
	region, err = h.user.Input("Region", suggestion)
	if err != nil {
		return "", err
	}
	if region == "" {
		return "", fmt.Errorf("region cannot be empty")
	}
	return region, nil
}

func (h contextCreateAWSHelper) askCredentials() (string, string, error) {
	/*confirm, err := h.user.Confirm("Enter AWS credentials", false)
	if err != nil {
		return "", "", err
	}
	if !confirm {
		return "", "", nil
	}*/

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
