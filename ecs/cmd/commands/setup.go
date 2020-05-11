package commands

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/docker/cli/cli-plugins/plugin"
	contextStore "github.com/docker/ecs-plugin/pkg/docker"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
)

const enterLabelPrefix = "Enter "

type setupOptions struct {
	name            string
	context         contextStore.AwsContext
	accessKeyID     string
	secretAccessKey string
}

func (s setupOptions) unsetRequiredArgs() []string {
	unset := []string{}
	if s.context.Profile == "" {
		unset = append(unset, "profile")
	}
	if s.context.Cluster == "" {
		unset = append(unset, "cluster")
	}

	if s.context.Region == "" {
		unset = append(unset, "region")
	}
	return unset
}

func SetupCommand() *cobra.Command {
	var opts setupOptions

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			//Override the root command PersistentPreRun
			//We just need to initialize the top parent command
			return plugin.PersistentPreRunE(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if requiredFlag := opts.unsetRequiredArgs(); len(requiredFlag) > 0 {
				if err := interactiveCli(&opts); err != nil {
					return err
				}
			}
			if opts.accessKeyID != "" && opts.secretAccessKey != "" {
				if err := saveCredentials(opts.context.Profile, opts.accessKeyID, opts.secretAccessKey); err != nil {
					return err
				}
			}
			return contextStore.NewContext(opts.name, &opts.context)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "aws", "Context Name")
	cmd.Flags().StringVarP(&opts.context.Profile, "profile", "p", "", "AWS Profile")
	cmd.Flags().StringVarP(&opts.context.Cluster, "cluster", "c", "", "ECS cluster")
	cmd.Flags().StringVarP(&opts.context.Region, "region", "r", "", "AWS region")
	cmd.Flags().StringVarP(&opts.accessKeyID, "aws-key-id", "k", "", "AWS Access Key ID")
	cmd.Flags().StringVarP(&opts.secretAccessKey, "aws-secret-key", "s", "", "AWS Secret Access Key")

	return cmd
}

func interactiveCli(opts *setupOptions) error {
	var section ini.Section

	if err := setContextName(opts); err != nil {
		return err
	}

	section, err := setProfile(opts, section)
	if err != nil {
		return err
	}

	if err := setCluster(opts, err); err != nil {
		return err
	}

	if err := setRegion(opts, section); err != nil {
		return err
	}

	if err := setCredentials(opts); err != nil {
		return err
	}

	return nil
}

func saveCredentials(profile string, accessKeyID string, secretAccessKey string) error {
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

func awsProfiles(filename string) (map[string]ini.Section, error) {
	profiles := map[string]ini.Section{"new profile": {}}
	if filename == "" {
		filename = defaults.SharedConfigFilename()
	}
	credIni, err := ini.Load(filename)
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

func setContextName(opts *setupOptions) error {
	if opts.name == "aws" {
		result, err := promptString(opts.name, "context name", enterLabelPrefix, 2)
		if err != nil {
			return err
		}
		opts.name = result
	}
	return nil
}

func setProfile(opts *setupOptions, section ini.Section) (ini.Section, error) {
	profilesList, err := awsProfiles("")
	if err != nil {
		return ini.Section{}, err
	}
	section, ok := profilesList[opts.context.Profile]
	if !ok {
		prompt := promptui.Select{
			Label: "Select AWS Profile",
			Items: reflect.ValueOf(profilesList).MapKeys(),
		}
		_, result, err := prompt.Run()
		if result == "new profile" {
			result, err := promptString(opts.context.Profile, "profile name", enterLabelPrefix, 2)
			if err != nil {
				return ini.Section{}, err
			}
			opts.context.Profile = result
		} else {
			section = profilesList[result]
			opts.context.Profile = result
		}
		if err != nil {
			return ini.Section{}, err
		}
	}
	return section, nil
}

func setRegion(opts *setupOptions, section ini.Section) error {
	defaultRegion := opts.context.Region
	if defaultRegion == "" && section.Name() != "" {
		region, err := section.GetKey("region")
		if err == nil {
			defaultRegion = region.Value()
		}
	}
	result, err := promptString(defaultRegion, "region", enterLabelPrefix, 2)
	if err != nil {
		return err
	}
	opts.context.Region = result
	return nil
}

func setCluster(opts *setupOptions, err error) error {
	result, err := promptString(opts.context.Cluster, "cluster name", enterLabelPrefix, 2)
	if err != nil {
		return err
	}
	opts.context.Cluster = result
	return nil
}

func setCredentials(opts *setupOptions) error {
	prompt := promptui.Prompt{
		Label:     "Enter credentials",
		IsConfirm: true,
	}
	_, err := prompt.Run()
	if err == nil {
		result, err := promptString(opts.accessKeyID, "AWS Access Key ID", enterLabelPrefix, 3)
		if err != nil {
			return err
		}
		opts.accessKeyID = result

		prompt = promptui.Prompt{
			Label:    "Enter AWS Secret Access Key",
			Validate: validateMinLen("AWS Secret Access Key", 3),
			Mask:     '*',
			Default:  opts.secretAccessKey,
		}
		result, err = prompt.Run()
		if err != nil {
			return err
		}
		opts.secretAccessKey = result
	}
	return nil
}

func promptString(defaultValue string, label string, labelPrefix string, minLength int) (string, error) {
	prompt := promptui.Prompt{
		Label:    labelPrefix + label,
		Validate: validateMinLen(label, minLength),
		Default:  defaultValue,
	}
	result, err := prompt.Run()
	if err != nil {
		return "", err
	}
	return result, nil
}

func validateMinLen(label string, minLength int) func(input string) error {
	return func(input string) error {
		if len(input) < minLength {
			return fmt.Errorf("%s must have more than %d characters", label, minLength)
		}
		return nil
	}
}
