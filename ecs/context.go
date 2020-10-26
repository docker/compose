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
}

func (c contextElements) HaveRequiredProps() bool {
	if c.AccessKey != "" && c.SecretKey != "" { //} && c.Region != "" {
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

func (h contextCreateAWSHelper) createContext(profile, region, description string) (interface{}, string) {
	if profile == "default" {
		profile = ""
	}
	description = strings.TrimSpace(
		fmt.Sprintf("%s (%s)", description, region))
	return store.EcsContext{
		Profile: profile,
		Region:  region,
	}, description
}

func (h contextCreateAWSHelper) ParseEnvVars(c *contextElements) {
	profile := os.Getenv("AWS_PROFILE")
	if profile != "" {
		c.Profile = profile
	}
	region := os.Getenv("AWS_REGION")
	if region != "" {
		c.Region = region
	}

	p := credentials.EnvProvider{}
	creds, err := p.Retrieve()
	if err != nil {
		return
	}
	c.AccessKey = creds.AccessKeyID
	c.SecretKey = creds.SecretAccessKey
	c.SessionToken = creds.SessionToken
}

func (h contextCreateAWSHelper) createContextData(_ context.Context, opts ContextParams) (interface{}, string, error) {
	creds := contextElements{}

	h.ParseEnvVars(&creds)

	options := []string{}
	if creds.HaveRequiredProps() {
		options = append(options, "existing AWS environment variables (detected credentials)")
	}
	options = append(options, "enter AWS credentials", "use an AWS profile or create a new profile (advanced)")
	selected, err := h.user.Select("Would you like to create your context based on", options)
	if err != nil {
		if err == terminal.InterruptErr {
			return nil, "", errdefs.ErrCanceled
		}
		return nil, "", err
	}

	if len(options) == 2 {
		selected = selected + 1
	}
	if selected != 0 {
		creds = contextElements{}
	}

	if creds.Region == "" {
		creds.Region = opts.Region
	}
	if creds.Profile == "" {
		creds.Profile = opts.Profile
	}

	switch selected {
	case 0:
		if creds.Region == "" {
			// prompt for the region to use with this context
			creds.Region, err = h.chooseRegion(creds.Region, creds.Profile)
			if err != nil {
				return nil, "", err
			}
		}
		if creds.Profile == "" {
			creds.Profile = opts.Name

		}
		fmt.Printf("Saving credentials under profile %s\n", creds.Profile)
		h.createProfile(creds.Profile, &creds)
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

	os.Exit(0)
	ecsCtx, descr := h.createContext(creds.Profile, creds.Region, opts.Description)
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
	options := []string{"new profile"}
	options = append(options, profiles...)

	selected, err := h.user.Select("Select AWS Profile", options)
	if err != nil {
		if err == terminal.InterruptErr {
			return "", errdefs.ErrCanceled
		}
		return "", err
	}
	profile := options[selected]
	if options[selected] == "new profile" {
		suggestion := ""
		if !contains(profiles, "default") {
			suggestion = "default"
		}
		name, err := h.user.Input("profile name", suggestion)
		if err != nil {
			return "", err
		}
		if name == "" {
			return "", fmt.Errorf("profile name cannot be empty")
		}
		return name, h.createProfile(name, nil)
	}
	return profile, nil
}

func (h contextCreateAWSHelper) chooseRegion(region string, profile string) (string, error) {
	suggestion := region
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
	if profile != "default" {
		profile = fmt.Sprintf("profile %s", profile)
	}
	section, err := configIni.GetSection(profile)
	if err != nil {
		if !strings.Contains(err.Error(), "does not exist") {
			return "", err
		}
		section, err = configIni.NewSection(profile)
		if err != nil {
			return "", err
		}
	}
	reg, err := section.GetKey("region")
	if err == nil {
		suggestion = reg.Value()
	}
	// promp user for region
	region, err = h.user.Input("Region", suggestion)
	if err != nil {
		return "", err
	}
	if region == "" {
		return "", fmt.Errorf("region cannot be empty")
	}
	// save selected/typed region under profile in ~/.aws/config
	_, err = section.NewKey("region", region)
	if err != nil {
		return "", err
	}
	return region, configIni.SaveTo(awsConfig)
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
