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
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/errdefs"
	"github.com/docker/compose-cli/prompt"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"gopkg.in/ini.v1"
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
	user             prompt.UI
	availableRegions func(opts *ContextParams) ([]string, error)
}

func newContextCreateHelper() contextCreateAWSHelper {
	return contextCreateAWSHelper{
		user:             prompt.User{},
		availableRegions: listAvailableRegions,
	}
}

func (h contextCreateAWSHelper) createContextData(_ context.Context, opts ContextParams) (interface{}, string, error) {
	if opts.CredsFromEnv {
		// Explicit creation from ENV variables
		ecsCtx, descr := h.createContext(&opts)
		return ecsCtx, descr, nil
	} else if opts.AccessKey != "" && opts.SecretKey != "" {
		// Explicit creation using keys
		err := h.createProfileFromCredentials(&opts)
		if err != nil {
			return nil, "", err
		}
	} else if opts.Profile != "" {
		// Excplicit creation by selecting a profile
		// check profile exists
		profilesList, err := getProfiles()
		if err != nil {
			return nil, "", err
		}
		if !contains(profilesList, opts.Profile) {
			return nil, "", errors.Wrapf(errdefs.ErrNotFound, "profile %q not found", opts.Profile)
		}
	} else {
		// interactive
		var options []string
		var actions []func(params *ContextParams) error

		if _, err := os.Stat(getAWSConfigFile()); err == nil {
			// User has .aws/config file, so we can offer to select one of his profiles
			options = append(options, "An existing AWS profile")
			actions = append(actions, h.selectFromLocalProfile)
		}

		options = append(options, "AWS secret and token credentials")
		actions = append(actions, h.createProfileFromCredentials)

		options = append(options, "AWS environment variables")
		actions = append(actions, func(params *ContextParams) error {
			opts.CredsFromEnv = true
			return nil
		})

		selected, err := h.user.Select("Create a Docker context using:", options)
		if err != nil {
			if err == terminal.InterruptErr {
				return nil, "", errdefs.ErrCanceled
			}
			return nil, "", err
		}

		err = actions[selected](&opts)
		if err != nil {
			return nil, "", err
		}
	}

	ecsCtx, descr := h.createContext(&opts)
	return ecsCtx, descr, nil
}

func (h contextCreateAWSHelper) createContext(c *ContextParams) (interface{}, string) {
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
	if opts.AccessKey == "" || opts.SecretKey == "" {
		fmt.Println("Retrieve or create AWS Access Key and Secret on https://console.aws.amazon.com/iam/home?#security_credential")
		accessKey, secretKey, err := h.askCredentials()
		if err != nil {
			return err
		}
		opts.AccessKey = accessKey
		opts.SecretKey = secretKey
	}

	if opts.Region == "" {
		err := h.chooseRegion(opts)
		if err != nil {
			return err
		}
	}
	// save as a profile
	if opts.Profile == "" {
		opts.Profile = "default"
	}
	// context name used as profile name
	err := h.saveCredentials(opts.Profile, opts.AccessKey, opts.SecretKey)
	if err != nil {
		return err
	}
	return h.saveRegion(opts.Profile, opts.Region)
}

func (h contextCreateAWSHelper) saveCredentials(profile string, accessKeyID string, secretAccessKey string) error {
	file := getAWSCredentialsFile()
	err := os.MkdirAll(filepath.Dir(file), 0700)
	if err != nil {
		return err
	}

	credentials, err := ini.Load(file)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		credentials = ini.Empty()
	}

	section, err := credentials.NewSection(profile)
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
	return credentials.SaveTo(file)
}

func (h contextCreateAWSHelper) saveRegion(profile, region string) error {
	if region == "" {
		return nil
	}
	// loads ~/.aws/config
	awsConfig := getAWSConfigFile()
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
		getAWSCredentialsFile(): false,
		getAWSConfigFile():      true,
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
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i] < profiles[j]
	})

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
	}
	if region == "" {
		// fallback to AWS default
		region = "us-east-1"
	}
	return region, nil
}

func (h contextCreateAWSHelper) chooseRegion(opts *ContextParams) error {
	regions, err := h.availableRegions(opts)
	if err != nil {
		return err
	}
	// promp user for region
	selected, err := h.user.Select("Region", regions)
	if err != nil {
		return err
	}
	opts.Region = regions[selected]
	return nil
}

func listAvailableRegions(opts *ContextParams) ([]string, error) {
	// Setup SDK with credentials, will also validate those
	session, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Credentials: credentials.NewStaticCredentials(opts.AccessKey, opts.SecretKey, ""),
			Region:      aws.String("us-east-1"),
		},
	})
	if err != nil {
		return nil, err
	}

	desc, err := ec2.New(session).DescribeRegions(&ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, err
	}
	var regions []string
	for _, r := range desc.Regions {
		regions = append(regions, aws.StringValue(r.RegionName))
	}
	return regions, nil
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

func getAWSConfigFile() string {
	awsConfig, ok := os.LookupEnv("AWS_CONFIG_FILE")
	if !ok {
		awsConfig = defaults.SharedConfigFilename()
	}
	return awsConfig
}

func getAWSCredentialsFile() string {
	awsConfig, ok := os.LookupEnv("AWS_SHARED_CREDENTIALS_FILE")
	if !ok {
		awsConfig = defaults.SharedCredentialsFilename()
	}
	return awsConfig
}
