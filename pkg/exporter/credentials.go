package exporter

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

type CredentialSource interface {
	Credentials() []*Credential
}

type Type string

const (
	GitHubApp Type = "gh-app"
	GitHubPAT Type = "gh-pat"
)

type (
	AppCredential struct {
		ID             int64  `yaml:"appId"`
		InstallationID int64  `yaml:"installationId"`
		Key            string `yaml:"key"`
	}

	PAT struct {
		Token string `yaml:"token"`
	}

	Credential struct {
		Type           Type `yaml:"type"`
		AppName        string
		*AppCredential `yaml:",inline"`
		*PAT           `yaml:",inline"`
	}
)

func (c *Credential) Name() string {
	return c.AppName
}

func (c *Credential) ID() int64 {
	return c.AppCredential.ID
}

func (c *Credential) InstallationID() int64 {
	return c.AppCredential.InstallationID
}

func (c *Credential) Base64PrivateKey() string {
	return c.AppCredential.Key
}

func (c *Credential) Token() string {
	return c.PAT.Token
}

func (c *Credential) Kind() string {
	return string(c.Type)
}

const FileCredentialFileName = "credentials.yml"

type FileCredentialSource struct {
	Data map[string]*Credential
}

func NewFileCredentialSource(fs *afero.Afero) (*FileCredentialSource, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	b, err := fs.ReadFile(filepath.Join(cwd, FileCredentialFileName))
	if err != nil {
		return nil, err
	}

	var src FileCredentialSource
	err = yaml.Unmarshal(b, &src)
	if err != nil {
		return nil, err
	}

	return &src, nil
}

func (src *FileCredentialSource) UnmarshalYAML(root *yaml.Node) error {
	credentials := make(map[string]*Credential)
	if err := root.Decode(credentials); err != nil {
		return err
	}

	src.Data = make(map[string]*Credential)

	for name, credential := range credentials {
		src.Data[name] = credential
		src.Data[name].AppName = name
	}

	return nil
}

func (src *FileCredentialSource) Credentials() []*Credential {
	credentials := make([]*Credential, 0, len(src.Data))
	for _, c := range src.Data {
		credentials = append(credentials, c)
	}

	return credentials
}
