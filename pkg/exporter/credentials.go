package exporter

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

type CredentialSource interface {
	Credentials() []*Credential
}

type (
	AppCredential struct {
		ID             int64  `json:"appId"`
		InstallationID int64  `json:"installationId"`
		Key            string `json:"key"`
	}

	PAT struct {
		Token string `json:"token"`
	}
)

type Type string

const (
	GitHubApp Type = "gh-app"
	GitHubPAT Type = "gh-pat"
)

type Credential struct {
	Type    Type `json:"type"`
	AppName string
	*AppCredential
	*PAT
}

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

const FileCredentialFileName = "credentials.json"

type FileCredentialSource struct {
	Data map[string]*Credential
}

func NewFileCredentialSource(fs afero.Fs) (*FileCredentialSource, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	b, err := afero.ReadFile(fs, filepath.Join(cwd, FileCredentialFileName))
	if err != nil {
		return nil, err
	}

	var conf FileCredentialSource
	err = json.Unmarshal(b, &conf)
	if err != nil {
		return nil, err
	}

	return &conf, nil
}

func (c *FileCredentialSource) UnmarshalJSON(data []byte) error {
	var m = make(map[string]*Credential)

	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	c.Data = make(map[string]*Credential)

	for name, credential := range m {
		c.Data[name] = credential
		c.Data[name].AppName = name
	}

	return nil
}

func (c *FileCredentialSource) Credentials() []*Credential {
	credentials := make([]*Credential, 0, len(c.Data))
	for _, c := range c.Data {
		credentials = append(credentials, c)
	}

	return credentials
}
