package tests

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	dockerConfigFile "github.com/docker/cli/cli/config/configfile"
	"github.com/docker/ecs-plugin/pkg/docker"
	"gotest.tools/v3/icmd"
)

var (
	e2ePath         = flag.String("e2e-path", ".", "Set path to the e2e directory")
	dockerCliPath   = os.Getenv("DOCKERCLI_BINARY")
	dockerCli       dockerCliCommand
	testContextName = "testAwsContextToBeRemoved"
)

type dockerCliCommand struct {
	path         string
	cliPluginDir string
}

type ConfigFileOperator func(configFile *dockerConfigFile.ConfigFile)

func (d dockerCliCommand) createTestCmd(ops ...ConfigFileOperator) (icmd.Cmd, func(), docker.AwsContext) {
	configDir, err := ioutil.TempDir("", "config")
	if err != nil {
		panic(err)
	}
	configFilePath := filepath.Join(configDir, "config.json")
	config := dockerConfigFile.ConfigFile{
		CLIPluginsExtraDirs: []string{
			d.cliPluginDir,
		},
		Filename: configFilePath,
	}
	for _, op := range ops {
		op(&config)
	}
	configFile, err := os.Create(configFilePath)
	if err != nil {
		panic(err)
	}
	defer configFile.Close()
	err = json.NewEncoder(configFile).Encode(config)
	if err != nil {
		panic(err)
	}

	awsContext := docker.AwsContext{
		Profile: "sandbox.devtools.developer",
		Region:  "eu-west-3",
	}
	testStore, err := docker.NewContextWithStore(testContextName, awsContext, filepath.Join(configDir, "contexts"))
	if err != nil {
		panic(err)
	}
	cleanup := func() {
		fmt.Println("cleanup")
		testStore.Remove(testContextName)
		os.RemoveAll(configDir)
	}
	env := append(os.Environ(),
		"DOCKER_CONFIG="+configDir,
		"DOCKER_CLI_EXPERIMENTAL=enabled") // TODO: Remove this once docker ecs plugin is no more experimental
	return icmd.Cmd{Env: env}, cleanup, awsContext
}

func (d dockerCliCommand) Command(args ...string) []string {
	return append([]string{d.path, "--context", testContextName}, args...)
}

func TestMain(m *testing.M) {
	flag.Parse()
	if err := os.Chdir(*e2ePath); err != nil {
		panic(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	dockerEcs := os.Getenv("DOCKERECS_BINARY")
	if dockerEcs == "" {
		dockerEcs = filepath.Join(cwd, "../dist/docker-ecs")
	}
	dockerEcs, err = filepath.Abs(dockerEcs)
	if err != nil {
		panic(err)
	}
	if dockerCliPath == "" {
		dockerCliPath = "docker"
	} else {
		dockerCliPath, err = filepath.Abs(dockerCliPath)
		if err != nil {
			panic(err)
		}
	}
	// Prepare docker cli to call the docker-ecs plugin binary:
	// - Create a symbolic link with the dockerEcs binary to the plugin directory
	cliPluginDir, err := ioutil.TempDir("", "configContent")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(cliPluginDir)
	createDockerECSSymLink(dockerEcs, cliPluginDir)

	dockerCli = dockerCliCommand{path: dockerCliPath, cliPluginDir: cliPluginDir}
	os.Exit(m.Run())
}

func createDockerECSSymLink(dockerEcs, configDir string) {
	dockerEcsExecName := "docker-ecs"
	if runtime.GOOS == "windows" {
		dockerEcsExecName += ".exe"
	}
	if err := os.Symlink(dockerEcs, filepath.Join(configDir, dockerEcsExecName)); err != nil {
		panic(err)
	}
}
