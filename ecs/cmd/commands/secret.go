package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/docker/cli/cli/command"
	"github.com/docker/ecs-plugin/pkg/amazon"
	"github.com/docker/ecs-plugin/pkg/docker"
	"github.com/spf13/cobra"
)

type createSecretOptions struct {
	Label string
}

type deleteSecretOptions struct {
	recover bool
}

func SecretCommand(dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manages secrets",
	}

	cmd.AddCommand(
		CreateSecret(dockerCli),
		InspectSecret(dockerCli),
		ListSecrets(dockerCli),
		DeleteSecret(dockerCli),
	)
	return cmd
}

func CreateSecret(dockerCli command.Cli) *cobra.Command {
	//opts := createSecretOptions{}
	cmd := &cobra.Command{
		Use:   "create NAME SECRET",
		Short: "Creates a secret.",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, args []string) error {
			client, err := amazon.NewClient(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return errors.New("Missing mandatory parameter: NAME")
			}
			name := args[0]
			secret := args[1]
			id, err := client.CreateSecret(context.Background(), name, secret)
			fmt.Println(id)
			return err
		}),
	}
	return cmd
}

func InspectSecret(dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect ID",
		Short: "Displays secret details",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, args []string) error {
			client, err := amazon.NewClient(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return errors.New("Missing mandatory parameter: ID")
			}
			id := args[0]
			secret, err := client.InspectSecret(context.Background(), id)
			if err != nil {
				return err
			}
			out, err := secret.ToJSON()
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		}),
	}
	return cmd
}

func ListSecrets(dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List secrets stored for the existing account.",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, args []string) error {
			client, err := amazon.NewClient(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			secrets, err := client.ListSecrets(context.Background())
			if err != nil {
				return err
			}

			printList(os.Stdout, secrets)
			return nil
		}),
	}
	return cmd
}

func DeleteSecret(dockerCli command.Cli) *cobra.Command {
	opts := deleteSecretOptions{}
	cmd := &cobra.Command{
		Use:     "delete NAME",
		Aliases: []string{"rm", "remove"},
		Short:   "Removes a secret.",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, args []string) error {
			client, err := amazon.NewClient(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return errors.New("Missing mandatory parameter: [NAME]")
			}
			return client.DeleteSecret(context.Background(), args[0], opts.recover)
		}),
	}
	cmd.Flags().BoolVar(&opts.recover, "recover", false, "Enable recovery.")
	return cmd
}

func printList(out io.Writer, secrets []docker.Secret) {
	printSection(out, len(secrets), func(w io.Writer) {
		for _, secret := range secrets {
			fmt.Fprintf(w, "%s\t%s\t%s\n", secret.ID, secret.Name, secret.Description)
		}
	}, "ID", "NAME", "DESCRIPTION")
}

func printSection(out io.Writer, len int, printer func(io.Writer), headers ...string) {
	w := tabwriter.NewWriter(out, 20, 1, 3, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	printer(w)
	w.Flush()
}
