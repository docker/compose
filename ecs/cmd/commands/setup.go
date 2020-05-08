package commands

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/docker/cli/cli-plugins/plugin"
	contextStore "github.com/docker/ecs-plugin/pkg/docker"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
)

func SetupCommand() *cobra.Command {
	var opts contextStore.AwsContext
	var name string
	var accessKeyID string
	var secretAccessKey string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			//Override the root command PersistentPreRun
			//We just need to initialize the top parent command
			return plugin.PersistentPreRunE(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if accessKeyID != "" && secretAccessKey != "" {
				if err := saveCredentials(opts.Profile, accessKeyID, secretAccessKey); err != nil {
					return err
				}
			}
			return contextStore.NewContext(name, &opts)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "aws", "Context Name")
	cmd.Flags().StringVarP(&opts.Profile, "profile", "p", "", "AWS Profile")
	cmd.Flags().StringVarP(&opts.Cluster, "cluster", "c", "", "ECS cluster")
	cmd.Flags().StringVarP(&opts.Region, "region", "r", "", "AWS region")
	cmd.Flags().StringVarP(&accessKeyID, "aws-key-id", "k", "", "AWS Access Key ID")
	cmd.Flags().StringVarP(&secretAccessKey, "aws-secret-key", "s", "", "AWS Secret Access Key")

	cmd.MarkFlagRequired("profile")
	cmd.MarkFlagRequired("cluster")
	cmd.MarkFlagRequired("region")
	return cmd
}

func saveCredentials(profile string, accessKeyID string, secretAccessKey string) error {
	p := credentials.SharedCredentialsProvider{Profile: profile}
	_, err := p.Retrieve()
	if err == nil {
		fmt.Println("credentials already exists!")
		return nil
	}
	if err.(awserr.Error).Code() == "SharedCredsLoad" {
		os.Create(p.Filename)
	}

	credIni, err := ini.Load(p.Filename)
	if err != nil {
		return err
	}
	section := credIni.Section(profile)
	section.Key("aws_access_key_id").SetValue(accessKeyID)
	section.Key("aws_secret_access_key").SetValue(secretAccessKey)

	credFile, err := os.OpenFile(p.Filename, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = credIni.WriteTo(credFile); err != nil {
		return err
	}
	return credFile.Close()
}
