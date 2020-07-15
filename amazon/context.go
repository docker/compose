package amazon

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/docker/api/context/store"
	"github.com/docker/api/prompt"
	"gopkg.in/ini.v1"
)

type contextCreateAWSHelper struct {
	user prompt.UI
}

func newContextCreateHelper() contextCreateAWSHelper {
	return contextCreateAWSHelper{
		user: prompt.User{},
	}
}

func (h contextCreateAWSHelper) createContextData(ctx context.Context, opts ContextParams) (interface{}, string, error) {

	accessKey := opts.AwsID
	secretKey := opts.AwsSecret

	awsCtx := store.AwsContext{
		Profile: opts.Profile,
		Cluster: opts.Cluster,
		Region:  opts.Region,
	}

	if h.missingRequiredFlags(awsCtx) {
		profilesList, err := h.getProfiles()
		if err != nil {
			return nil, "", err
		}
		// get profile
		_, ok := profilesList[awsCtx.Profile]
		if !ok {
			profile, err := h.chooseProfile(profilesList)
			if err != nil {
				return nil, "", err
			}
			awsCtx.Profile = profile
		}
		// set cluster
		cluster, err := h.chooseCluster(awsCtx.Cluster)
		if err != nil {
			return nil, "", err
		}
		awsCtx.Cluster = cluster
		// set region
		region, err := h.chooseRegion(awsCtx.Region, profilesList[awsCtx.Profile])
		if err != nil {
			return nil, "", err
		}
		awsCtx.Region = region

		accessKey, secretKey, err = h.askCredentials()
		if err != nil {
			return nil, "", err
		}
	}
	if accessKey != "" && secretKey != "" {
		if err := h.saveCredentials(awsCtx.Profile, accessKey, secretKey); err != nil {
			return nil, "", err
		}
	}

	description := fmt.Sprintf("%s@%s", awsCtx.Cluster, awsCtx.Region)
	if opts.Description != "" {
		description = fmt.Sprintf("%s (%s)", opts.Description, description)
	}

	return awsCtx, description, nil
}

func (h contextCreateAWSHelper) missingRequiredFlags(ctx store.AwsContext) bool {
	if ctx.Profile == "" || ctx.Region == "" {
		return true
	}
	return false
}

func (h contextCreateAWSHelper) saveCredentials(profile string, accessKeyID string, secretAccessKey string) error {
	p := credentials.SharedCredentialsProvider{Profile: profile}
	_, err := p.Retrieve()
	if err == nil {
		fmt.Println("credentials already exists!")
		return nil
	}

	if err.(awserr.Error).Code() == "SharedCredsLoad" && err.(awserr.Error).Message() == "failed to load shared credentials file" {
		os.Create(p.Filename)
	}

	credIni, err := ini.Load(p.Filename)
	if err != nil {
		return err
	}
	section, err := credIni.NewSection(profile)
	if err != nil {
		return err
	}
	section.NewKey("aws_access_key_id", accessKeyID)
	section.NewKey("aws_secret_access_key", secretAccessKey)
	return credIni.SaveTo(p.Filename)
}

func (h contextCreateAWSHelper) getProfiles() (map[string]ini.Section, error) {
	profiles := map[string]ini.Section{"new profile": {}}
	credIni, err := ini.Load(defaults.SharedConfigFilename())
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	for _, section := range credIni.Sections() {
		if strings.HasPrefix(section.Name(), "profile") {
			profiles[section.Name()[len("profile "):]] = *section
		}
	}
	return profiles, nil
}

func (h contextCreateAWSHelper) chooseProfile(section map[string]ini.Section) (string, error) {
	keys := reflect.ValueOf(section).MapKeys()
	profiles := make([]string, len(keys))
	for i := 0; i < len(keys); i++ {
		profiles[i] = keys[i].String()
	}

	selected, err := h.user.Select("Select AWS Profile", profiles)
	if err != nil {
		if err == terminal.InterruptErr {
			os.Exit(0)
		}
		return "", err
	}
	profile := profiles[selected]

	if profiles[selected] == "new profile" {

		return h.user.Input("profile name", "")
	}
	return profile, nil
}

func (h contextCreateAWSHelper) chooseRegion(region string, section ini.Section) (string, error) {
	defaultRegion := region
	if defaultRegion == "" && section.Name() != "" {
		reg, err := section.GetKey("region")
		if err == nil {
			defaultRegion = reg.Value()
		}
	}
	result, err := h.user.Input("Region", defaultRegion)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (h contextCreateAWSHelper) chooseCluster(cluster string) (string, error) {
	if cluster == "" {
		cluster = "default"
	}
	result, err := h.user.Input("Cluster name", cluster)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (h contextCreateAWSHelper) askCredentials() (string, string, error) {
	confirm, err := h.user.Confirm("Enter credentials", false)
	if err != nil {
		return "", "", err
	}
	if confirm {
		accessKeyID, err := h.user.Input("AWS Access Key ID", "")
		if err != nil {
			return "", "", err
		}
		secretAccessKey, err := h.user.Password("Enter AWS Secret Access Key")
		if err != nil {
			return "", "", err
		}
		// validate password
		if len(secretAccessKey) < 3 {
			return "", "", fmt.Errorf("AWS Secret Access Key must have more than 3 characters")
		}
		return accessKeyID, secretAccessKey, nil
	}
	return "", "", nil
}
