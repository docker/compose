package convert

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
)

// GetRunVolumes return volume configurations for a project and a single service
// this is meant to be used as a compose project of a single service
func GetRunVolumes(volumes []string) (map[string]types.VolumeConfig, []types.ServiceVolumeConfig, error) {
	var serviceConfigVolumes []types.ServiceVolumeConfig
	projectVolumes := make(map[string]types.VolumeConfig, len(volumes))
	for i, v := range volumes {
		var vi volumeInput
		err := vi.parse(fmt.Sprintf("volume-%d", i), v)
		if err != nil {
			return nil, nil, err
		}
		projectVolumes[vi.name] = types.VolumeConfig{
			Name:   vi.name,
			Driver: azureFileDriverName,
			DriverOpts: map[string]string{
				volumeDriveroptsAccountNameKey: vi.username,
				volumeDriveroptsAccountKeyKey:  vi.key,
				volumeDriveroptsShareNameKey:   vi.share,
			},
		}
		sv := types.ServiceVolumeConfig{
			Type:   azureFileDriverName,
			Source: vi.name,
			Target: vi.target,
		}
		serviceConfigVolumes = append(serviceConfigVolumes, sv)
	}

	return projectVolumes, serviceConfigVolumes, nil
}

type volumeInput struct {
	name     string
	username string
	key      string
	share    string
	target   string
}

func scapeKeySlashes(rawURL string) (string, error) {
	urlSplit := strings.Split(rawURL, "@")
	if len(urlSplit) < 1 {
		return "", errors.New("invalid url format " + rawURL)
	}
	userPasswd := strings.ReplaceAll(urlSplit[0], "/", "_")
	scaped := userPasswd + rawURL[strings.Index(rawURL, "@"):]

	return scaped, nil
}

func unscapeKey(passwd string) string {
	return strings.ReplaceAll(passwd, "_", "/")
}

// Removes the second ':' that separates the source from target
func volumeURL(pathURL string) (*url.URL, error) {
	scapedURL, err := scapeKeySlashes(pathURL)
	if err != nil {
		return nil, err
	}
	pathURL = "//" + scapedURL

	count := strings.Count(pathURL, ":")
	if count > 2 {
		return nil, fmt.Errorf("unable to parse volume mount %q", pathURL)
	}
	if count == 2 {
		tokens := strings.Split(pathURL, ":")
		pathURL = fmt.Sprintf("%s:%s%s", tokens[0], tokens[1], tokens[2])
	}
	return url.Parse(pathURL)
}

func (v *volumeInput) parse(name string, s string) error {
	volumeURL, err := volumeURL(s)
	if err != nil {
		return fmt.Errorf("volume specification %q could not be parsed %q", s, err)
	}
	v.username = volumeURL.User.Username()
	if v.username == "" {
		return fmt.Errorf("volume specification %q does not include a storage username", v)
	}
	passwd, ok := volumeURL.User.Password()
	if !ok || passwd == "" {
		return fmt.Errorf("volume specification %q does not include a storage key", v)
	}
	v.key = unscapeKey(passwd)
	v.share = volumeURL.Host
	if v.share == "" {
		return fmt.Errorf("volume specification %q does not include a storage file share", v)
	}
	v.name = name
	v.target = volumeURL.Path
	if v.target == "" {
		v.target = filepath.Join("/run/volumes/", v.share)
	}
	return nil
}
